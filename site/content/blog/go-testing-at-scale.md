---
title: "Why Your Go Tests Are Slow (and What to Do About It)"
date: 2026-07-11
description: "Most slow Go test suites aren't slow because of slow tests. Sequential execution, redundant setup, and shared state are what prevent parallelism."
tags: ["Performance"]
keywords: ["why are my go tests slow", "speed up go tests", "go test parallelism", "go test t.parallel", "go test slow"]
cta_text: "Run your slowest suites as parallel processes."
toc: true
faq:
  - q: "Why are my Go tests slow even though each test is fast?"
    a: "Usually it is structure, not the tests. Every test in a package runs in one process, sequentially by default, so the longest single-package run sets your ceiling no matter how many CPU cores you have."
  - q: "Does t.Parallel() speed up Go tests?"
    a: "Only for genuinely independent tests. The moment tests share a database, filesystem, or package-level variable, t.Parallel() creates races and flaky failures instead of speed."
  - q: "How do I stop reconnecting to the database in every test?"
    a: "TestMain gives one per-package hook and sync.Once gives lazy caching, but both are manual wiring and sync.Once can be poisoned by t.Fatal. A per-process fixture that sets up once is the structural fix."
  - q: "Does splitting a Go package make tests faster?"
    a: "Yes — each package compiles to its own test binary and runs as a separate process, so package-level parallelism kicks in. But splitting purely for speed creates package boundaries that don't match your domains."
aliases: ["/blog/go-testing-at-scale.html"]
---

`go test ./...` takes 90 seconds. You've profiled the hot tests and they're fast. The database setup is cached. The assertions are trivial. So where does the time go?

Usually the answer isn't slow tests. It's slow structure: the way tests are organized determines how much work runs sequentially, how much setup gets repeated, and how much parallelism the runtime can actually use. This post looks at the three structural patterns that make Go test suites slow, and what to do about each one. The payoff is real: the benchmark later in the post takes the same example suite from 6 seconds to 0.5 seconds through structural changes alone, without touching a single test.

## How `go test` parallelism actually works

Before diagnosing anything, it helps to understand what `go test` does under the hood. There are two levels of parallelism, controlled by two different flags:

- **Package-level parallelism (`-p`).** `go test` compiles each package into a separate test binary and runs up to `-p` packages concurrently. The default is `GOMAXPROCS`, which is typically the number of CPU cores. If you have 8 packages, up to 8 run at the same time.
- **Test-level parallelism (`-parallel`).** Within a single package's test binary, `go test` runs tests sequentially by default. Tests that call `t.Parallel()` are released to run concurrently, up to `-parallel` goroutines (default: `GOMAXPROCS`).

The key insight: within a package, tests run in a single process. All test functions, all subtests, all suite methods: one binary, one process. The only parallelism available is goroutine-level, via `t.Parallel()`.

This means the ceiling for your test suite's speed is determined by the longest single-package execution. If one package has 200 tests that run sequentially, that package is your bottleneck, no matter how many CPU cores you have.

## Problem 1: The single-process bottleneck

Consider a package with 60 test functions: 20 for users, 20 for orders, 20 for payments:

```go {title="service_test.go"}
func TestCreateUser(t *testing.T)    { /* ... */ }
func TestGetUser(t *testing.T)       { /* ... */ }
func TestDeleteUser(t *testing.T)    { /* ... */ }
// ... 17 more user tests

func TestCreateOrder(t *testing.T)   { /* ... */ }
// ... 19 more order tests

func TestProcessPayment(t *testing.T) { /* ... */ }
// ... 19 more payment tests
```

Each test takes 100ms (database setup + a few assertions). The total wall-clock time for this package is:

```diagram
60 tests x 100ms = 6 seconds (sequential)
```

All 60 tests run in one process, one after another. Even though the user tests are logically independent from the payment tests, they share a process and execute sequentially. On an 8-core machine, 7 cores sit idle while this package grinds through its queue.

You can add `t.Parallel()` to each test function, and for tests that don't share state, you should. But the moment tests touch a shared database, a shared filesystem, or package-level variables, `t.Parallel()` creates races instead of solving them. More on that in Problem 3.

> If you use testify/suite, this problem is sharper. `suite.Run` executes each test method sequentially, and there's no way to call `t.Parallel()` on individual methods from within the suite. The suite runner itself can be marked parallel with respect to other top-level test functions, but its internal methods remain sequential.

The most effective stdlib-only fix is splitting the package. Three packages means three binaries, three processes, and package-level parallelism kicks in. But splitting by parallelism rather than by domain creates artificial package boundaries that make the codebase harder to navigate.

## Problem 2: Redundant fixture setup

The second structural bottleneck is rebuilding expensive resources for every test. Here's a typical pattern:

```go
func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("postgres", testDSN)  // lazy: no connection yet
    if err != nil {
        t.Fatal(err)
    }
    db.Exec("TRUNCATE users")                 // 70ms: TCP connect + truncate
    t.Cleanup(func() { db.Close() })
    return db
}

func TestCreateUser(t *testing.T) {
    db := setupTestDB(t)  // 70ms
    svc := NewUserService(db)
    // ... test logic (5ms)
}

func TestGetUser(t *testing.T) {
    db := setupTestDB(t)  // 70ms again
    svc := NewUserService(db)
    // ... test logic (5ms)
}
```

70ms of setup, 20 test functions. That's 1.4 seconds of pure connection churn. Scale to 60 tests across your package and you're spending over 4 seconds just establishing database connections.

The stdlib offers two escape hatches:

- **`TestMain`** gives you a per-package setup hook. You can connect once and assign to a package-level variable. But `TestMain` is per package: one function for all tests. If different groups of tests need different setup, you're back to per-test setup or awkward conditional logic.
- **`sync.Once`** gives you lazy initialization. Wrap the connection in a `sync.Once` and every test that needs it gets the cached result. This works, but it's manual wiring. Each fixture needs its own `Once`, its own variable, and its own teardown logic, typically deferred from `TestMain`.

Neither approach handles dependencies between fixtures. If the database connection depends on a container, and the container depends on a Docker socket check, you're writing a dependency graph by hand in `TestMain`.

## Problem 3: `t.Parallel()` and shared state

The most underused tool in Go's testing package is `t.Parallel()`. It marks a test as safe to run concurrently with other parallel tests. In theory, this is how you speed up a package with many independent tests.

In practice, it collides with shared state. A common pattern is a package-level variable set up in `TestMain`:

```go
var testDB *sql.DB

func TestMain(m *testing.M) {
    testDB = connectTestDB()
    os.Exit(m.Run())
}

func TestCreateUser(t *testing.T) {
    t.Parallel()
    testDB.Exec("INSERT INTO users ...")  // concurrent write
}

func TestDeleteUser(t *testing.T) {
    t.Parallel()
    testDB.Exec("DELETE FROM users ...")  // concurrent write
}
```

Both tests write to the same `testDB`, and likely to the same table. You get flaky tests, deadlocks, or data races. The fix is to give each parallel test its own connection, which means giving up the shared setup that `TestMain` was supposed to simplify.

> In test suites (testify/suite or similar), the struct itself is the shared state. Every method reads and writes through `s.db`, `s.svc`, etc. There's no built-in way to give each parallel method its own copy of the struct. You'd need to restructure the entire suite to pass resources through function parameters rather than struct fields, losing the organizational benefits of a suite.

There's also a subtler `t.Parallel()` gotcha with subtests and closures:

```go
func TestUsers(t *testing.T) {
    users := []string{"alice", "bob", "carol"}
    for _, u := range users {
        t.Run(u, func(t *testing.T) {
            t.Parallel()
            testUser(t, u)  // all three goroutines see "carol"
        })
    }
}
```

The loop variable `u` is captured by reference. By the time the parallel subtests execute, the loop has finished and `u` is `"carol"` for all three. Go 1.22 fixed this specific issue by making loop variables per-iteration, but the broader point stands: `t.Parallel()` with closures requires careful reasoning about what state is shared and when it's accessed.

## What you can do today

Before changing tools, there are structural improvements you can make with the standard library and your existing framework. The zero-code first step: check that `-p` and `-parallel` aren't pinned below your core count (some CI templates set them low) and tune them before touching any test code. Beyond that:

### Profile before optimizing

Run `go test -v -count=1 ./...` and look at the timings. Often one or two packages dominate. Within those packages, add `-run` filters to isolate which tests are slow. Profile CPU time with `-cpuprofile` if you suspect computation, or just time the setup functions. Keep CI wall-clock time separate from your day-to-day feedback loop, though: the loop you run fifty times a day is worth optimizing on its own terms, and [Go Test Watch Mode and Focused Tests]({{< ref "/blog/the-inner-loop" >}}) covers that side.

### Split large packages

The single most effective optimization is splitting a large package into smaller ones. Each package becomes a separate test binary that runs in its own process. If you have 4 logical groups of tests averaging 5 seconds each, moving them to separate packages turns 20 seconds of sequential execution into 5 seconds of parallel execution (on a 4+ core machine).

The trade-off is package boundaries that exist for parallelism, not for domain clarity. Sometimes these align (each domain gets its own package). Sometimes they don't.

### Use `sync.Once` for expensive setup

If multiple tests in the same package need the same database connection, share it:

```go
var (
    testDB     *sql.DB
    testDBOnce sync.Once
)

func getTestDB(t *testing.T) *sql.DB {
    testDBOnce.Do(func() {
        var err error
        testDB, err = sql.Open("postgres", testDSN)
        if err != nil {
            t.Fatal(err)  // dangerous: poisons the Once for all subsequent callers
        }
    })
    return testDB
}
```

This avoids reconnecting per test, but has a subtle flaw. `t.Fatal` calls `runtime.Goexit()`, which terminates the goroutine while running deferred functions, including the one inside `sync.Once` that marks the initialization as "done." The `Once` is now permanently poisoned: subsequent callers skip the init function entirely and get a `nil` `testDB`, crashing with nil-pointer panics that have no connection to the original setup failure. And teardown requires a `TestMain` to close the connection after all tests finish.

### Use `t.Parallel()` where it's safe

For tests that are genuinely independent (each creates its own state and doesn't read shared mutable data), adding `t.Parallel()` is free speed. This works best with table-driven tests where each case is self-contained:

```go
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        result := process(tt.input)
        if result != tt.want {
            t.Errorf("got %v, want %v", result, tt.want)
        }
    })
}
```

This is ideal for pure functions with no external dependencies. It's less practical for integration tests that touch a database, a filesystem, or a network service.

## Process isolation: parallel Go test suites by default

The optimizations above are real and worth doing. But they work *around* a fundamental constraint: all tests in a package share a single process. Parallelism within that process is opt-in, manual, and fragile.

What if the constraint were removed? What if each suite ran in its own OS process, with its own memory space, and parallelism between suites was the default rather than the exception?

This is the approach [gotest](https://github.com/mvrahden/go-test) takes. Each test suite is compiled into the package's test binary (via `go test -c`), but each suite is **executed as a separate subprocess** with a `-test.run` filter that targets only that suite. The result is process-level isolation between suites:

```diagram
go test (package)
├── process 1: UserSuite      (20 tests)
├── process 2: OrderSuite     (20 tests)    ← all three run concurrently
└── process 3: PaymentSuite   (20 tests)
```

The three suites from the earlier example now run in parallel by default. No `t.Parallel()`, no careful state management, no package splitting. The OS guarantees memory isolation. A panic in `PaymentSuite` cannot crash `UserSuite`. A goroutine leak in one process does not affect the others.

The concurrency budget defaults to `2 x GOMAXPROCS`, split between inter-suite parallelism (number of concurrent processes) and intra-suite parallelism (goroutines within each process). On an 8-core machine, up to 8 suite processes run concurrently, each with its own goroutine budget for method-level parallelism.

### Method-level parallelism

Within a suite, you can opt into parallel test methods:

```go
type UserServiceTestSuite struct{}

type UserServiceCtx struct {
    db  *TestDB
    svc *UserService
}

func (s *UserServiceTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.SuiteConfig{Parallel: true}
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) *UserServiceCtx {
    db := NewTestDB(t.T())
    return &UserServiceCtx{db: db, svc: NewUserService(db)}
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T, ctx *UserServiceCtx) {
    err := ctx.svc.Create("alice@example.com")
    gotest.NoError(t, err)
}

func (s *UserServiceTestSuite) TestDelete(t *gotest.T, ctx *UserServiceCtx) {
    ctx.svc.Create("alice@example.com")
    err := ctx.svc.Delete("alice@example.com")
    gotest.NoError(t, err)
}
```

The key is the returning `BeforeEach`. It creates a per-test context: a separate struct holding the resources each test needs. The generated code calls `BeforeEach` before each parallel method, giving it a fresh `ctx` with its own `db` and `svc`. No shared state, no races, no manual `t.Parallel()` calls. [Go Test Lifecycle]({{< ref "/blog/go-test-lifecycle" >}}) covers where the returning `BeforeEach` fits in the full execution order.

### Fixture lifecycle

Instead of `sync.Once` and package-level variables, gotest manages fixture lifecycles through a dependency DAG, the structural answer to Problem 2's redundant setup. A fixture is a struct with conventional method names:

```go
type DatabaseFixture struct {
    Container *PostgresContainer
    DB        *sql.DB
}

func (f *DatabaseFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()
}

func (f *DatabaseFixture) BeforeAll(t *gotest.T) {
    f.Container = startPostgres(t.T())
    f.DB = connectDB(f.Container.DSN())
}

func (f *DatabaseFixture) AfterAll(t *gotest.T) {
    f.DB.Close()
    f.Container.Stop()
}
```

`BeforeAll` runs once per suite process, not once per test. All test methods in the suite share the fixture. Dependencies between fixtures are expressed as pointer fields: if `DatabaseFixture` has a `*DockerFixture` field, the Docker fixture sets up first. The generator resolves the DAG automatically. [Advanced Go Test Fixtures]({{< ref "/blog/advanced-fixture-patterns" >}}) goes deeper into DAG design and fixture composition.

For cross-package sharing, a `SharedFixture` runs in its own setup process. Its state is serialized to JSON and deserialized in each suite process via `Hydrate()`. A Postgres container starts once, and every suite across every package connects to it. [More on fixture patterns]({{< ref "/blog/test-fixtures-in-go" >}}), and [Sharing Test Fixtures Across Go Packages]({{< ref "/blog/shared-fixtures" >}}) for the deep treatment of cross-package sharing.

## Benchmark: 6 seconds to 0.5 seconds

The three-suite example from earlier, on an 8-core machine:

{{< terminal title="comparison" >}}
<span class="t-dim">sequential (single process):</span>
  3 suites × 20 tests × 100ms = <span class="t-fail">6.0s</span>

<span class="t-dim">parallel suites (3 processes):</span>
  max(20 × 100ms, 20 × 100ms, 20 × 100ms) = <span class="t-warn">2.0s</span>

<span class="t-dim">parallel suites + parallel methods (3 processes, 4 goroutines each):</span>
  max(20/4 × 100ms, ...) ≈ <span class="t-pass">0.5s</span>
{{< /terminal >}}

The improvement from 6 seconds to 0.5 seconds comes entirely from structural changes: process isolation between suites and goroutine parallelism within them. No test logic was changed. No assertion was added or removed. The test code is identical; only the execution model is different.

These are simplified numbers. Real workloads have uneven test durations, fixture setup costs, and I/O contention. But the pattern holds: structural parallelism is multiplicative. `t.Parallel()` on individual tests is additive.

## Where to start

If your test suite is slow, the diagnostic path is straightforward:

1. **Find the bottleneck package.** Run `go test -v -count=1 ./...` and sort by duration. One or two packages usually dominate.
1. **Check for sequential suites.** If the bottleneck package has multiple suites, they're running sequentially. Split the package, or use a tool that isolates suites into separate processes.
1. **Check for redundant setup.** If your setup helper does expensive I/O (connect to database, start container, load fixtures), look at how many times it runs. Consider shared fixtures that set up once per process or once per test run.
1. **Check for parallelism blockers.** If tests are independent but run sequentially because they share state, the test structure is preventing parallelism. Either restructure for `t.Parallel()` or use a framework that provides per-test isolation.

The fastest test suite is the one where isolation is structural (guaranteed by processes and memory boundaries) rather than behavioral (relying on developers to avoid sharing state). The first kind scales with cores. The second kind scales with discipline.
