---
title: "Go Test Lifecycle: Setup, Teardown, and Execution Order"
date: 2026-07-14
description: "Go test setup and teardown, in order: suite hooks, fixture hooks, cleanup guarantees, and retries — the full gotest lifecycle from BeforeAll to AfterAll."
tags: ["Patterns"]
keywords: ["go test setup teardown", "go test lifecycle", "beforeeach aftereach go", "go test cleanup order"]
toc: true
cta_text: "See these lifecycle guarantees in your own suite."
faq:
  - q: "Does AfterEach run after t.Fatal?"
    a: "Yes. AfterEach is deferred within each test's scope, so it runs even if the test method calls t.Fatal, panics, or fails an assertion that stops execution."
  - q: "Can AfterAll run if BeforeAll panics?"
    a: "Yes. AfterAll is registered via t.Cleanup before BeforeAll executes, so it runs even if BeforeAll panics or calls t.Fatal. Guard against partial initialization with nil checks."
  - q: "In what order do fixture and suite hooks run?"
    a: "Fixture hooks wrap suite hooks: Fixture.BeforeAll, then Suite.BeforeAll; per test, Fixture.BeforeEach, Suite.BeforeEach, the test, Suite.AfterEach, Fixture.AfterEach; finally Suite.AfterAll, then Fixture.AfterAll."
  - q: "Can BeforeAll be retried?"
    a: "Fixture BeforeAll can be retried via FixtureConfig: on error the runtime waits RetryDelay and calls BeforeAll again, up to Retries times, with Timeout applying to each attempt. The retry applies only to BeforeAll."
---

Every test framework has setup and teardown. The interesting question is not *whether* they exist, but what guarantees the lifecycle gives you. Can `AfterAll` run if `BeforeAll` panics? Does `AfterEach` still execute on `t.Fatal`? Where do fixture hooks fit relative to suite hooks? If you have ever guessed at these answers while debugging a leaked database connection, this post is for you.

Other posts in this series cover [fixture patterns]({{< ref "/blog/test-fixtures-in-go" >}}) and [getting started with suites]({{< ref "/blog/zero-to-suite" >}}). This post is different. It maps the full lifecycle from the inside out, so you build the correct mental model once and stop guessing.

## The four suite lifecycle hooks

A gotest suite can implement up to four lifecycle hooks. All are optional. An unimplemented hook is a no-op.

- **`BeforeAll`** runs once before the first test method in the suite.
- **`AfterAll`** runs once after the last test method completes.
- **`BeforeEach`** runs before every test method.
- **`AfterEach`** runs after every test method.

Each hook accepts either `*gotest.T` or `*testing.T`. You can mix them within the same suite. A `BeforeEach` that takes `*gotest.T` and a `TestCreate` that takes `*testing.T` is perfectly valid.

The ordering is what you would expect:

```diagram
BeforeAll
├── BeforeEach → Test A → AfterEach
├── BeforeEach → Test B → AfterEach
└── BeforeEach → Test C → AfterEach
AfterAll
```

So far, nothing surprising. The interesting parts are in the guarantees.

## The cleanup guarantee

This is the most important lifecycle property in gotest, and the one most likely to differ from your intuition.

**`AfterAll` is registered via `t.Cleanup` before `BeforeAll` executes.** This is a deliberate design choice, not an implementation detail. It means `AfterAll` runs even if `BeforeAll` panics or calls `t.Fatal`. The Go test runner guarantees that `t.Cleanup` functions run regardless of how a test terminates, and gotest leverages that guarantee.

**`AfterEach` is deferred within each test's scope.** It runs even if the test method calls `t.Fatal`, panics, or fails an assertion that stops execution. The defer mechanism in the generated code ensures this.

Why does this matter? Because tests that allocate real resources need deterministic cleanup. A database connection that leaks because `BeforeAll` panicked after `sql.Open` but before the rest of setup completed is a real problem. A container that is never terminated because a test called `t.Fatal` before reaching the cleanup code is a CI pipeline that slowly runs out of memory.

```go {title="suite_test.go"}
type DatabaseTestSuite struct {
    container *PostgresContainer
    db        *sql.DB
}

func (s *DatabaseTestSuite) BeforeAll(t *gotest.T) {
    s.container = startPostgresContainer(t)
    s.db = connectDB(t, s.container.ConnectionString())
}

func (s *DatabaseTestSuite) AfterAll(t *gotest.T) {
    // Runs even if BeforeAll panicked after starting the container
    // but before connecting to the database.
    if s.db != nil {
        s.db.Close()
    }
    if s.container != nil {
        s.container.Terminate(context.Background())
    }
}
```

The nil checks in `AfterAll` are the key pattern. Because `AfterAll` is guaranteed to run regardless of what happened in `BeforeAll`, you guard against partial initialization. The container might have started but the database connection might not exist yet. The cleanup code handles both cases.

## Returning BeforeEach: per-test isolation

When `BeforeEach` returns a value, that value is passed as a parameter to the test method and to `AfterEach`. Each test gets its own instance. This is the mechanism that makes method-level parallelism safe.

```go {title="suite_test.go"}
type UserServiceTestSuite struct {
    DB *DatabaseFixture
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) *TestCtx {
    tx := s.DB.BeginTx(t)
    return &TestCtx{
        Tx:      tx,
        Service: NewUserService(tx),
    }
}

func (s *UserServiceTestSuite) AfterEach(t *gotest.T, ctx *TestCtx) {
    ctx.Tx.Rollback()
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T, ctx *TestCtx) {
    t.When("email is valid", func(t *gotest.T) {
        user, err := ctx.Service.Create("alice@example.com")
        t.It("creates the user", func(t *gotest.T) {
            gotest.NoError(t, err)
            gotest.Equal(t, "alice@example.com", user.Email)
        })
    })
}
```

The `ctx` parameter is unique per test. `TestCreate` gets its own transaction and its own service instance. If the suite is configured with `SuiteConfig{Parallel: true}`, tests run concurrently without data races because they share no mutable state. Each test writes to its own transaction that is rolled back in `AfterEach`, so no test's writes are visible to any other test.

This is the non-obvious power of the returning `BeforeEach` pattern: it turns method-level parallelism from a coordination problem into a non-problem. There is nothing to coordinate when each test has its own world.

## Fixture lifecycle: wrapping suites

Fixtures have their own set of lifecycle hooks: `BeforeAll`, `AfterAll`, `BeforeEach`, and `AfterEach`. These wrap the suite's hooks, forming an outer layer:

```diagram
Fixture.BeforeAll
  └── Suite.BeforeAll
        ├── Fixture.BeforeEach → Suite.BeforeEach → Test → Suite.AfterEach → Fixture.AfterEach
        ├── Fixture.BeforeEach → Suite.BeforeEach → Test → Suite.AfterEach → Fixture.AfterEach
        └── ...
  └── Suite.AfterAll
Fixture.AfterAll
```

There is an important signature difference. Fixture hooks use `(ctx context.Context) error`, not `*gotest.T`. This is because fixtures operate at a different level than tests. They manage infrastructure, not assertions. A fixture's `BeforeAll` opens a database connection; it does not make test assertions. The `context.Context` parameter carries timeouts and cancellation; the `error` return lets the framework handle failures with retries and structured error reporting.

```go {title="fixtures.go"}
type DatabaseFixture struct {
    DB    *sql.DB
    store map[string]any
}

func (f *DatabaseFixture) BeforeAll(ctx context.Context) error {
    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        return err
    }
    f.DB = db
    return nil
}

func (f *DatabaseFixture) AfterAll(ctx context.Context) error {
    return f.DB.Close()
}

func (f *DatabaseFixture) BeforeEach(ctx context.Context) error {
    _, err := f.DB.ExecContext(ctx, "BEGIN")
    return err
}

func (f *DatabaseFixture) AfterEach(ctx context.Context) error {
    _, err := f.DB.ExecContext(ctx, "ROLLBACK")
    return err
}
```

The fixture's `BeforeEach` begins a transaction before each test, and `AfterEach` rolls it back afterward. The suite's own `BeforeEach` and test method run inside that transaction. This gives you per-test isolation at the database level without any coordination between the fixture and the suite.

## Fixture composition: stacking lifecycles

When one fixture depends on another, their lifecycles nest. Dependencies are expressed as pointer fields on the fixture struct:

```go {title="fixtures.go"}
type ServiceFixture struct {
    DB    *DatabaseFixture
    Cache *CacheFixture
}
```

The generator resolves the dependency graph at build time and orders startup accordingly. Leaf dependencies start first, composed fixtures start after their dependencies are ready, and teardown happens in reverse:

```diagram
DatabaseFixture.BeforeAll ──┐
CacheFixture.BeforeAll ────┤
                           └── ServiceFixture.BeforeAll
                                 └── Suite.BeforeAll
                                       └── tests...
                                 └── Suite.AfterAll
                           └── ServiceFixture.AfterAll
CacheFixture.AfterAll ─────┘
DatabaseFixture.AfterAll ──┘
```

The graph resolution is static. The generator reads the struct fields, builds a DAG, topologically sorts it, and emits the startup and teardown calls in the correct order. If there is a cycle, the generator reports it at build time, not at test execution time.

## BeforeAll retry support

Fixtures that depend on external services face transient failures. A container might take longer to start than expected. A network connection might fail on the first attempt. gotest supports retry configuration for fixture `BeforeAll` through the `FixtureConfig` method:

```go {title="fixtures.go"}
func (f *DatabaseFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()
    // 5-minute timeout, 1 retry with 5s delay
}
```

For custom values:

```go {title="fixtures.go"}
func (f *DatabaseFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.FixtureConfig{
        Timeout:    3 * time.Minute,
        Retries:    2,
        RetryDelay: 10 * time.Second,
    }
}
```

The retry applies only to `BeforeAll`. If a fixture's `BeforeAll` returns an error, the runtime waits for `RetryDelay`, then calls `BeforeAll` again, up to `Retries` times. The `Timeout` applies to each individual attempt, enforced through the `context.Context` passed to the hook. The default configuration (`DefaultFixtureConfig`) has a 2-minute timeout with no retries. `ContainerFixtureConfig` extends this to 5 minutes with 1 retry and a 5-second delay, tuned for container startup times.

## The complete timeline

Here is the full lifecycle of a test run, from start to finish, with a composed fixture. This is the mental model to internalize:

1. **Generator resolves the fixture dependency graph** (build time). Struct fields form a DAG. Topological sort determines startup order. Cycles are rejected.
1. **Test binary starts.** `go test` compiles and runs the generated code.
1. **Leaf fixtures' `BeforeAll` runs** (in dependency order). `DatabaseFixture.BeforeAll` before `ServiceFixture.BeforeAll`. Each gets a `context.Context` with the configured timeout. Failures are retried according to `FixtureConfig`.
1. **Composed fixtures' `BeforeAll` runs** (after their dependencies). `ServiceFixture.BeforeAll` can safely reference `f.DB.DB` because the `DatabaseFixture` is already initialized.
1. **Suite's `BeforeAll` runs.** The suite can reference fixture fields that were populated in the fixture's `BeforeAll`.
1. **For each test method:**
   1. Fixture's `BeforeEach` (outer layer)
   1. Suite's `BeforeEach` (returns `ctx` if applicable)
   1. Test method executes (receives `ctx` if `BeforeEach` returned one)
   1. Suite's `AfterEach` (receives `ctx`)
   1. Fixture's `AfterEach` (outer layer)
1. **Suite's `AfterAll` runs** (via `t.Cleanup`, guaranteed).
1. **Composed fixtures' `AfterAll` runs.**
1. **Leaf fixtures' `AfterAll` runs** (reverse dependency order). `DatabaseFixture.AfterAll` runs last because other fixtures may still reference its resources during their own teardown.

The per-test steps repeat for every test method. If the suite uses `SuiteConfig{Parallel: true}` with a returning `BeforeEach`, the test methods run concurrently, each with their own `ctx` from the suite's `BeforeEach` and their own fixture `BeforeEach`/`AfterEach` wrapping.

> The cleanup guarantee applies at every level. Fixture `AfterAll` runs even if the suite's `BeforeAll` panics. Suite `AfterEach` runs even if the test method calls `t.Fatal`. The generated code registers teardown before calling setup, so the cleanup path is always in place before anything can go wrong.

## When to use which hook

The lifecycle gives you four levels of setup and teardown, split across suites and fixtures. Choosing the right one is a matter of cost and scope:

- **`BeforeAll` / `AfterAll`** are for expensive, shared resources. Database connections, container startup, service initialization. These run once and are amortized across all tests in the suite. If setup takes more than a few milliseconds, it probably belongs here.
- **`BeforeEach` / `AfterEach`** are for per-test isolation. Transactions, temp directories, fresh state. These ensure each test starts clean. The cost must be low enough to pay on every test.
- **Returning `BeforeEach`** is for when tests need isolated context *and* you want method-level parallelism. The returned value gives each test its own state object, making concurrent execution safe without locks.
- **Fixture hooks** are for infrastructure that multiple suites share. If two suites both need a Postgres connection, extract the connection management into a `DatabaseFixture` with its own `BeforeAll`/`AfterAll`. The suites reference the fixture; the fixture manages the resource.

A common pattern combines these levels: a fixture's `BeforeAll` starts a database, the fixture's `BeforeEach` begins a transaction, the suite's returning `BeforeEach` creates service instances using that transaction, and the fixture's `AfterEach` rolls back. Each level handles one concern.

## The lifecycle in one example

The lifecycle is designed so that each layer can be reasoned about independently. Fixtures do not need to know about suite hooks. Suites do not need to know about fixture composition. The generator wires them together in the correct order, and the cleanup guarantee ensures that teardown happens regardless of how tests terminate.

If you remember one thing from this post, make it this: **teardown is registered before setup runs.** That single property is what makes the entire lifecycle reliable. Everything else follows from it.

For the fixture patterns that this lifecycle supports, see [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}). For sharing fixtures across package boundaries, see [Shared Fixtures]({{< ref "/blog/shared-fixtures" >}}). For the full API surface, see the [reference documentation]({{< ref "/reference" >}}).
