---
title: "Test Fixtures in Go: Patterns That Scale"
date: 2026-07-07
description: "Every growing Go project needs shared test infrastructure: databases, caches, containers. A look at fixture patterns from global vars to DAG-based lifecycle management."
tag: "Patterns"
readTime: 12
aliases: ["/blog/test-fixtures-in-go.html"]
---

Most Go projects eventually need shared test infrastructure: a database connection, a message queue, a container that takes seconds to start. The stdlib doesn't have a built-in answer for this. So every team invents its own fixture pattern, and most of those patterns stop working at some point.

This post looks at the common approaches, where each one breaks down, and what a fixture system needs to handle the cases that real projects run into.

## The problem

A test fixture is any shared resource that tests depend on. The simplest example: a database that multiple test functions query. The fixture problem is about lifecycle: when does the database get created, when does it get torn down, and how do you make sure tests don't step on each other?

The challenge grows along two axes:

- **Cost.** Some fixtures are cheap (an in-memory map). Others are expensive (spinning up a Postgres container). Expensive fixtures can't be created per-test; you need to share them.
- **Scope.** Some fixtures belong to one test file. Others are shared across an entire package. Some need to be shared across multiple packages. Each scope has different lifecycle requirements.

## Pattern 1: Global variables

The most common first approach:

```go
var testDB *sql.DB

func TestMain(m *testing.M) {
    var err error
    testDB, err = sql.Open("postgres", os.Getenv("TEST_DB_URL"))
    if err != nil {
        log.Fatal(err)
    }
    defer testDB.Close()
    os.Exit(m.Run())
}

func TestCreateUser(t *testing.T) {
    _, err := testDB.Exec("INSERT INTO users ...")
    // ...
}

func TestListUsers(t *testing.T) {
    rows, err := testDB.Query("SELECT * FROM users")
    // depends on what TestCreateUser left behind
}
```

`TestMain` is Go's package-level setup hook: it runs once before any tests in the package. The database connection lives in a global variable that every test function can access.

This works until it doesn't:

- **Tests share state.** `TestListUsers` sees rows that `TestCreateUser` inserted. Tests become order-dependent. Add `t.Parallel()` and they become race-condition-dependent.
- **Teardown is fragile.** If any test panics, `defer testDB.Close()` might not run. If `TestMain` itself fails, the process exits with no cleanup.
- **One fixture per package.** `TestMain` can only be defined once. Need a database *and* a cache? Everything goes into one function.
- **No per-test reset.** There's no hook to truncate tables between tests. You'd need to write that into every test function manually.

## Pattern 2: Helper functions

The next step is to wrap fixture creation in a helper:

```go
func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := sql.Open("postgres", os.Getenv("TEST_DB_URL"))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })
    return db
}

func TestCreateUser(t *testing.T) {
    db := setupTestDB(t)
    // db is fresh for this test
}

func TestListUsers(t *testing.T) {
    db := setupTestDB(t)
    // db is fresh for this test too
}
```

Better. Each test gets its own connection. `t.Cleanup` handles teardown even if the test panics. No shared state between tests.

But the cost problem is back: if the fixture is expensive (a container, a schema migration, a seed dataset), creating it per-test is too slow. And if you add caching to amortize the cost, say a `sync.Once` around the container startup, you've reinvented the global variable pattern with extra steps.

## Pattern 3: Struct-based fixtures with sync.Once

A more structured approach uses a struct to hold the fixture state and `sync.Once` to ensure one-time initialization:

```go
type DBFixture struct {
    once sync.Once
    db   *sql.DB
    err  error
}

func (f *DBFixture) Get(t *testing.T) *sql.DB {
    t.Helper()
    f.once.Do(func() {
        f.db, f.err = sql.Open("postgres", os.Getenv("TEST_DB_URL"))
    })
    if f.err != nil {
        t.Fatal(f.err)
    }
    return f.db
}

var dbFixture DBFixture

func TestCreateUser(t *testing.T) {
    db := dbFixture.Get(t)
    // ...
}
```

This solves the cost problem: the database is created once and shared. But it still has the state-leaking problem (tests share the same database), and teardown is still unclear: when does the database get closed? `sync.Once` has no corresponding "undo."

More fundamentally, none of these patterns handle **fixture dependencies**. What if your test needs a database *and* a cache, and the cache needs the database connection to configure itself? Now you need to manage initialization order, and that's where hand-rolled patterns start to collapse.

## What fixtures actually need

Looking at the patterns above, a fixture system needs to handle four concerns:

1. **Lifecycle hooks.** Setup and teardown at both the suite level (once) and the test level (per-test). Fixtures that take `context.Context` and return `error`, so expensive operations can be cancelled and failures can be handled.
1. **Dependency ordering.** If fixture B depends on fixture A, A's setup must complete before B's begins. Teardown runs in reverse order. This is a DAG (directed acyclic graph) problem.
1. **Scope control.** Some fixtures are package-scoped (shared across suites in one package). Some are shared across multiple packages. The fixture system needs to understand these scopes.
1. **Isolation.** Per-test hooks that reset state between test methods, so tests don't inherit side effects from each other.

## Fixtures as structs with lifecycle conventions

In [gotest](https://github.com/mvrahden/go-test), a fixture is a struct with naming conventions, the same pattern used for test suites. A struct whose name ends in `Fixture` is recognized as a package-scoped fixture. Its lifecycle methods are discovered by name:

```go {title="fixtures.go"}
type DatabaseFixture struct {
    DB *sql.DB
}

func (f *DatabaseFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()  // 5m timeout, 1 retry
}

func (f *DatabaseFixture) BeforeAll(_ context.Context) error {
    var err error
    f.DB, err = sql.Open("postgres", os.Getenv("TEST_DB_URL"))
    return err
}

func (f *DatabaseFixture) AfterAll(_ context.Context) error {
    return f.DB.Close()
}

func (f *DatabaseFixture) BeforeEach(_ context.Context) error {
    _, err := f.DB.Exec("TRUNCATE users, orders")
    return err
}
```

A few things to notice:

- **Lifecycle methods take `context.Context` and return `error`.** This is different from suite lifecycle hooks (which take `*gotest.T`). Fixtures represent infrastructure; they need cancellation and error propagation.
- **`BeforeAll`/`AfterAll`** run once for the package. The database is opened once and closed once.
- **`BeforeEach`** runs before every test method in every suite that uses this fixture. Tables get truncated, so each test starts clean.
- **`FixtureConfig`** controls timeout and retry behavior. `ContainerFixtureConfig()` gives a 5-minute timeout with one retry, appropriate for infrastructure that might need time to start.

## Suites consume fixtures by referencing them

A suite declares its fixture dependency by including it as a pointer field:

```go {title="suite_test.go"}
type UserRepositoryTestSuite struct {
    DB   *DatabaseFixture
    repo *userRepository
}

func (s *UserRepositoryTestSuite) BeforeEach(t *gotest.T) {
    s.repo = newUserRepository(s.DB.DB)
}

func (s *UserRepositoryTestSuite) TestCreateUser(t *gotest.T) {
    t.When("the input is valid", func(w *gotest.T) {
        err := s.repo.Create(User{Name: "Alice"})

        w.It("succeeds", func(it *gotest.T) {
            gotest.NoError(it, err)
        })
    })
}
```

The field `DB *DatabaseFixture` is the entire dependency declaration. gotest sees the pointer to a `Fixture`-suffixed type and wires it up automatically. By the time `BeforeEach` runs on the suite, the fixture's `BeforeAll` has already completed and `s.DB.DB` is a live database connection.

The lifecycle order for each test method is:

1. Fixture `BeforeEach`: truncate tables
1. Suite `BeforeEach`: create repository
1. Test method: run the test
1. Suite `AfterEach`
1. Fixture `AfterEach`

If multiple suites in the same package reference `*DatabaseFixture`, they all share the same instance. `BeforeAll` runs once for the package, not once per suite.

## Fixture dependencies form a DAG

Fixtures can depend on other fixtures. A cache fixture that needs a database connection declares the dependency the same way, as a pointer field:

```go
type CacheFixture struct {
    DB    *DatabaseFixture  // depends on DatabaseFixture
    Cache *redis.Client
}

func (f *CacheFixture) BeforeAll(_ context.Context) error {
    f.Cache = redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    // f.DB.DB is already initialized. DatabaseFixture.BeforeAll ran first
    return f.Cache.Ping(context.Background()).Err()
}
```

gotest resolves these dependencies into a directed acyclic graph and runs `BeforeAll` in topological order. Independent fixtures (no dependency between them) initialize concurrently. Teardown runs in reverse topological order.

```diagram
DatabaseFixture.BeforeAll → CacheFixture.BeforeAll → tests run → CacheFixture.AfterAll → DatabaseFixture.AfterAll
```

This scales to arbitrary depth. If a third fixture depends on the cache, it slots into the graph naturally. You never manually coordinate initialization order; the dependency pointers encode it.

## The cross-package problem

Everything above works within a single package. But `go test` runs each package as a **separate OS process**. There is no shared memory between `pkg/user` and `pkg/order`. They are different binaries, running in different processes, with different address spaces.

This means none of the patterns above can share a fixture across packages:

- **`TestMain`** runs once per package, not once per test run. Five packages that need Postgres means five containers started, five schema migrations run.
- **`sync.Once`** is a per-process primitive. It does not cross process boundaries.
- **Helper packages** like `testutil.GetDB()` get imported by each package independently. Each one initializes its own connection in its own process.

The stdlib has no answer for this. Most teams fall back to external orchestration: a Makefile or CI script starts the database before `go test`, passes the DSN through an environment variable, and each package's `TestMain` connects independently. It works, but the fixture lifecycle lives outside of Go, in shell scripts, docker-compose files, or CI configuration. The test code cannot express "I need a Postgres instance" as a dependency; it can only assume one already exists.

## Sharing fixtures across packages

gotest has **shared fixtures** for this: structs whose name ends in `SharedFixture`. A shared fixture runs in its own subprocess, once for the entire test run. Its state is serialized as JSON and transferred to every test package that needs it.

```go {title="shared_fixtures.go"}
type PostgresSharedFixture struct {
    DSN    string   // serialized: transferred to test packages
    conn   *sql.DB  // local: reconstructed by Hydrate
}

func (f *PostgresSharedFixture) BeforeAll(_ context.Context) error {
    // start container, run migrations
    f.DSN = "postgres://localhost:5432/testdb"
    var err error
    f.conn, err = sql.Open("postgres", f.DSN)
    return err
}

func (f *PostgresSharedFixture) AfterAll(_ context.Context) error {
    f.conn.Close()
    // stop container
    return nil
}

func (f *PostgresSharedFixture) Hydrate(_ context.Context) error {
    var err error
    f.conn, err = sql.Open("postgres", f.DSN)
    return err
}

func (f *PostgresSharedFixture) Dehydrate(_ context.Context) error {
    return f.conn.Close()
}
```

The key concept is the split between **transfer fields** and **local fields**:

- **Transfer fields** (`DSN`) are exported and serialized as JSON. They cross process boundaries.
- **Local fields** (`conn`) are unexported or assigned in `Hydrate`. They represent resources that can't be serialized: connections, file handles, goroutines.

`Hydrate` runs in each test package's process after the JSON state is deserialized. It reconstructs the non-serializable resources from the transferred data. `Dehydrate` cleans up those local resources when the test package finishes.

## Where this leaves you

Every fixture pattern represents a trade-off between simplicity, cost, and isolation:

- **Global variables** are simple but share state and have fragile teardown.
- **Helper functions** isolate well but duplicate expensive setup.
- **sync.Once wrappers** amortize cost but reintroduce shared state and have no lifecycle management.
- **All of the above** are confined to a single package. None can share a database or container across the process boundary that `go test` puts between packages.

A struct-based fixture system with lifecycle hooks, dependency ordering, scope control, and serialization-based shared fixtures addresses all four concerns. The fixture is created once (cheap), torn down reliably (lifecycle hooks with context and error handling), each test gets a clean slate (per-test hooks), and expensive infrastructure like databases and containers can be shared across the process boundaries that `go test` puts between packages.

The important part isn't the specific implementation. It is recognizing that fixture management is a graph problem. Once you have more than two fixtures that depend on each other, manual coordination becomes a liability. A system that resolves dependencies, enforces initialization order, handles teardown in reverse, and bridges the cross-package process boundary is worth the upfront investment.
