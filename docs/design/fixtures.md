# Fixtures

> **Normative.** Part of the design spec (see `spec.md`) — update these docs before
> derived material.

Fixtures replace `TestMain` + package-level singletons with convention-driven setup.
Any struct whose name ends in `Fixture` is a package fixture; any struct ending in `SharedFixture` is a cross-package shared fixture.

## Package Fixture (`*Fixture` suffix)

A package fixture runs `BeforeAll` once per suite process, then injects state into child test suites via named pointer fields.
Each suite is dispatched in its own test process, so suites binding the same package fixture each get a fresh instance — package fixtures share code, not runtime state, across suites.

```go
// fixture_test.go

type E2ESetupFixture struct {
    Pool      *pgxpool.Pool
    ServerURL string
    OrgID     uuid.UUID

    container testcontainers.Container // unexported: teardown handle, not injected state
}

func (f *E2ESetupFixture) BeforeAll(ctx context.Context) error {
    pg, err := testhelper.StartPostgres(ctx)
    if err != nil {
        return err
    }
    f.container = pg.Container
    f.Pool = pg.Pool
    // ... wire API, seed fixtures ...
    return nil
}

func (f *E2ESetupFixture) AfterAll(ctx context.Context) error {
    f.Pool.Close()
    return f.container.Terminate(ctx)
}
```

```go
// batch_test.go

type BatchTestSuite struct {
    Fixture *E2ESetupFixture
}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    // s.Fixture.Pool is populated by E2ESetupFixture.BeforeAll
}
```

### Rules

- A package may define multiple fixtures. Suites can reference any combination of them.
- Fixtures can depend on multiple other fixtures, forming a DAG (directed acyclic graph).
  If two fixtures share a common ancestor type, that ancestor is instantiated once (diamond deduplication).
- The fixture struct must have `BeforeAll(ctx context.Context) error`.
  `AfterAll(ctx context.Context) error` is optional.
- `BeforeEach(ctx context.Context) error` and `AfterEach(ctx context.Context) error` are optional and run around every test case in all child suites.
- Setup runs in topological order (dependencies first; independent fixtures in parallel).
  Teardown runs in reverse topological order.
- TestSuites reference fixtures via named pointer fields (`Fixture *E2ESetupFixture`).
  A suite may have multiple fixture fields.
- Fixtures need no `TestMain` of yours: generated code declares the test binary's `TestMain` when the package binds fixtures.
  A `TestMain` you keep runs the tests through `gotestruntime.Main(m)` in place of `m.Run()`, or passes `gotestruntime.M(m)` to a library that runs them itself (`goleak.VerifyTestMain`, `testscript.Main`) — fixtures tear down inside either.
  Otherwise a fixture-bound test fails before anything is set up, naming the fix; so does a test run again after `Main` returned. The `testmain-fixture-teardown` lint rule reports the same at the source and rewrites the call.

### Generated test output

Fixture setup runs automatically before the first fixture-bound test — fixture names do not appear in test paths.
Suites bound to a fixture produce the same test names as standalone suites:

```
TestBatchTestSuite/TestDispatch        PASS
TestKeyTestSuite/TestCreate            PASS
```

Filter with `-run TestBatchTestSuite` to run only batch tests.

## Fixture Dependencies

Fixtures can reference other fixtures via named pointer fields to form a DAG.
Setup runs in topological order (dependencies first; independent fixtures in parallel).
Teardown runs in reverse topological order.

### Single parent (simple chain)

```go
type InfraFixture struct {
    Pool *pgxpool.Pool
}

func (f *InfraFixture) BeforeAll(ctx context.Context) error { /* start containers */ return nil }
func (f *InfraFixture) AfterAll(ctx context.Context) error  { /* terminate containers */ return nil }

type APIFixture struct {
    Infra     *InfraFixture
    ServerURL string
}

func (f *APIFixture) BeforeAll(ctx context.Context) error {
    srv := api.NewServer(":0", api.Dependencies{Pool: f.Infra.Pool})
    f.ServerURL = srv.URL
    return nil
}

type ReconcilerTestSuite struct { Infra *InfraFixture }  // only needs DB
type BatchTestSuite struct { API *APIFixture }            // needs full API
```

### Multiple parents

A fixture can depend on multiple other fixtures:

```go
type DatabaseFixture struct {
    Pool *pgxpool.Pool
}

func (f *DatabaseFixture) BeforeAll(ctx context.Context) error { /* start postgres */ return nil }
func (f *DatabaseFixture) AfterAll(ctx context.Context) error  { /* terminate postgres */ return nil }

type CacheFixture struct {
    Client *redis.Client
}

func (f *CacheFixture) BeforeAll(ctx context.Context) error { /* start redis */ return nil }
func (f *CacheFixture) AfterAll(ctx context.Context) error  { /* terminate redis */ return nil }

type ServiceFixture struct {
    DB        *DatabaseFixture
    Cache     *CacheFixture
    ServerURL string
}

func (f *ServiceFixture) BeforeAll(ctx context.Context) error {
    // f.DB.Pool and f.Cache.Client are already initialized
    srv := service.New(f.DB.Pool, f.Cache.Client)
    f.ServerURL = srv.URL
    return nil
}
```

Setup order: `DatabaseFixture` and `CacheFixture` run in parallel, then `ServiceFixture`.

### Diamond deduplication

When two fixtures share a common ancestor, the ancestor is instantiated once:

```go
type InfraFixture struct { Pool *pgxpool.Pool }

type APIFixture struct {
    Infra     *InfraFixture
    ServerURL string
}

type WorkerFixture struct {
    Infra    *InfraFixture
    QueueURL string
}

type IntegrationTestSuite struct {
    API    *APIFixture
    Worker *WorkerFixture
}
```

Both `APIFixture` and `WorkerFixture` depend on `InfraFixture`.
The framework creates one `InfraFixture` instance and injects it into both.

### Suites with multiple fixtures

A suite can reference multiple fixtures directly:

```go
type OrderTestSuite struct {
    API   *APIFixture
    Cache *CacheFixture
}
```

### Generated test output

```
TestReconcilerTestSuite/TestOrphan  PASS
TestBatchTestSuite/TestDispatch     PASS
```

Filter with `-run TestReconcilerTestSuite` to run only reconciler tests.

## Shared Fixtures (`*SharedFixture` suffix)

Shared fixtures run in a subprocess managed by the `gotest` CLI.
They start once per CLI invocation and are shared across all packages.
State crosses the process boundary via JSON serialization, with `Hydrate` handling reconstruction of non-serializable resources.

```go
// tests/fixtures/postgres.go

type PostgresSharedFixture struct {
    ConnStr string
    Port    int
    Pool    *pgxpool.Pool
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error {
    c, err := postgres.Run(ctx, "postgres:16")
    if err != nil {
        return err
    }
    f.ConnStr = c.MustConnectionString(ctx)
    f.Port = c.MappedPort(ctx, "5432").Int()
    return f.connect(ctx)
}

func (f *PostgresSharedFixture) AfterAll(ctx context.Context) error {
    f.Pool.Close()
    return nil
}

func (f *PostgresSharedFixture) Hydrate(ctx context.Context) error {
    return f.connect(ctx)
}

func (f *PostgresSharedFixture) Dehydrate(ctx context.Context) error {
    f.Pool.Close()
    return nil
}

func (f *PostgresSharedFixture) connect(ctx context.Context) error {
    var err error
    f.Pool, err = pgxpool.New(ctx, f.ConnStr)
    return err
}
```

### Rules

- Shared fixture types must live in a **non-internal** package (not under `internal/`).
  The setup subprocess compiles outside the module tree and cannot import `internal/` paths.
  Shared fixtures may freely import and depend on `internal/` packages — only the fixture type's own package path is restricted.
- `BeforeAll(ctx context.Context) error` and `AfterAll(ctx context.Context) error` — runs in the subprocess.
- `Hydrate(ctx context.Context) error` and `Dehydrate(ctx context.Context) error` — runs in the test process, optional.
- `BeforeEach`/`AfterEach` are not allowed on shared fixtures.
- Exported fields are serialized as JSON and transferred to the test process via a state file (`GOTEST_SHARED_STATE_FILE`).
- Fields assigned in `Hydrate` (directly, or in receiver methods called from `Hydrate`, one level deep) are **local** — excluded from serialization and reconstructed by `Hydrate`.

### State transfer

In the example above, `ConnStr` and `Port` are transferable (not assigned in `Hydrate`'s call chain).
`Pool` is local (assigned in `connect()`, which is called from `Hydrate`).

Convention: in `Hydrate`, assign to local fields.
Read transferred fields but do not reassign them — use local variables for any transformation.

### Using shared fixtures in test suites

Shared fixtures work with both standalone suites (no package fixture) and fixture-bound suites.

**Standalone suites** reference shared fixtures via named pointer fields — `Pool` is available after `Hydrate` runs:

```go
type UserTestSuite struct {
    Postgres *fixtures.PostgresSharedFixture
}

func (s *UserTestSuite) TestCreate(t *gotest.T) {
    // s.Postgres.Pool is a real, local *pgxpool.Pool — created by Hydrate
}
```

A standalone suite can reference multiple shared fixtures.
Suites without any shared fixture fields coexist in the same package without issue.

**Fixture-bound suites** wire shared fixtures through a package fixture to add package-specific resources:

```go
type E2ESetupFixture struct {
    Postgres  *fixtures.PostgresSharedFixture
    ServerURL string
}

func (f *E2ESetupFixture) BeforeAll(ctx context.Context) error {
    srv := api.NewServer(":0", api.Dependencies{Pool: f.Postgres.Pool})
    f.ServerURL = srv.URL
    return nil
}
```

### SharedFixture Dependencies

SharedFixtures can depend on other SharedFixtures via pointer fields — the same pattern used by package fixtures:

```go
type PostgresSharedFixture struct {
    ConnStr string
    Pool    *pgxpool.Pool
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error {
    c, err := postgres.Run(ctx, "postgres:16")
    if err != nil {
        return err
    }
    f.ConnStr = c.MustConnectionString(ctx)
    return f.connect(ctx)
}

func (f *PostgresSharedFixture) Hydrate(ctx context.Context) error { return f.connect(ctx) }

func (f *PostgresSharedFixture) Dehydrate(ctx context.Context) error {
    f.Pool.Close()
    return nil
}

type SchemaSharedFixture struct {
    Postgres *PostgresSharedFixture   // dependency — Postgres starts first
    Version  string
}

func (f *SchemaSharedFixture) BeforeAll(ctx context.Context) error {
    // f.Postgres.ConnStr is available — Postgres.BeforeAll already completed
    return migrate(f.Postgres.ConnStr)
}
```

#### Rules

- Dependencies are expressed via `*XSharedFixture` pointer fields on the struct.
- `BeforeAll` runs in dependency order: parents before children, independent fixtures in parallel.
- Cyclic dependencies are rejected at resolution time.
- SharedFixtures cannot depend on PackageFixtures (they run in different processes) — a package-fixture pointer field on a shared fixture is a generation error.
- A shared fixture without `BeforeAll`, or with only one of `Hydrate`/`Dehydrate`, is a generation error — including fixtures reached transitively from other packages.

#### Per-suite dispatch

Each shared fixture's state is emitted immediately after its `BeforeAll` completes (streaming protocol).
Suites are dispatched as soon as their specific shared fixture dependencies are ready — they do not wait for all shared fixtures to finish.

If a suite needs `SchemaSharedFixture`, and `SchemaSharedFixture` depends on `PostgresSharedFixture`, both are included automatically (transitive dependencies).
Per-suite state files contain only the entries that suite needs.

### CLI flow

```
gotest ./tests/e2e ./tests/integration -v
```

1. Load target packages, collect test suites and fixtures from AST
2. Resolve fixtures demand-driven: walk the type graph from targeted suites to discover all required package and shared fixtures (including cross-package)
3. If shared fixtures are needed, generate and start a setup subprocess (calls `BeforeAll`, serializes transferable fields as JSON to stdout)
4. Generate test code for each package
5. Run `go test` — test harness deserializes fixture state, calls `Hydrate` if present
6. Send SIGTERM to the setup subprocess (calls `AfterAll` in reverse order)

## Execution Model

### Never speculative

Shared fixtures are never speculative: a shared fixture is resident exactly while a scheduled suite needs it.
Before anything starts, the runner computes the set of suites the run will dispatch — after `-run` filtering and config-level skips — and starts only the shared fixtures some scheduled suite requires, closed over the dependency DAG.
A fixture nothing scheduled needs is never started, and can therefore never fail the run.
When fixtures are skipped this way, the runner emits one debug line to stderr: `gotest: N shared fixture(s) not started (no scheduled suite requires them)`.

### Window scheduling

A run dispatches in two phases: the parallel bulk, then the serial tail of `Exclusive` suites.
Each phase has an alive set — `Alive(phase)` is the DAG-closure of the union of the shared fixture keys the phase's suites require — and a shared fixture is resident exactly for the phases that need it:

- Fixtures in neither alive set never start.
- `Alive(bulk)` starts up-front, concurrent with package compilation.
- At the bulk→tail barrier — every parallel suite drained, no exclusive suite dispatched yet — the runner tears down `Alive(bulk) ∖ Alive(tail)` in reverse dependency order, then starts `Alive(tail) ∖ Alive(bulk)` in dependency order, under the same per-fixture retry and budget policy as the up-front phase.
- Run-end teardown releases whatever is still resident. Fixtures released at the barrier are skipped there: every fixture's teardown has exactly one owner.

The barrier speaks two verbs to the setup subprocess over its stdin — start and teardown, each naming a set of state keys — acknowledged on stdout after any late state lines.
`Alive(tail)` is computed from the tail suites that actually became runnable, so a fixture whose only exclusive suite failed to compile is released at the barrier, not started for nothing.
On cancellation the barrier is skipped entirely and run-end teardown owns everything.
Window scheduling is always on; there is no configuration.

### Lazy setup, teardown after the tests

Fixture setup does not run at package init.
The first fixture-bound test, benchmark or fuzz target triggers it (`gotestruntime.FixtureOnce`), so a run whose `-run`/`-skip` selects no fixture-bound unit never pays it.
Teardown runs once `m.Run()` returns in the generated `TestMain`, after every test the run selected — whatever `-run`, `-skip`, `-count` or `-shuffle` it was given.
A panicking test never returns from `m.Run()`: the testing package runs its cleanups and ends the process, so the fixtures are released in those cleanups, after the suite's `AfterAll`. The same holds for a panicking benchmark, fuzz body, `It`, `When` or `AfterAll`.
An interrupt releases them too: Ctrl-C, gotest's `--timeout`, a cancelled run. The process tears its fixtures down, then ends by the signal, and the runner waits for it within the teardown budget — past `test2json`, which ends first. Waiting for the whole process tree also waits for a child process a test left running that ignores the interrupt, up to that budget; a second Ctrl-C ends the CLI at once.
Under the runner a signal counts only once the runner has announced the stop, so a signal the code under test sends its own process is left to that code. Under plain `go test` only Ctrl-C (SIGINT) counts. Fuzzing is left to Go, which stops on Ctrl-C by itself — stopping the workers, saving what it found — and returns from `m.Run()`, so the fixtures tear down after it; `gotest fuzz` stops its fuzzing processes with the same Ctrl-C.
A panic on a goroutine the test started outside `gotest.Go`, a panic in a plain `func TestX` beside the suites under plain `go test` (gotest generates nothing around it), a process killed outright, or `go test -timeout` expiring ends the process without any of this, and nothing is released. A resource that must never leak belongs in a shared fixture or behind a reaper of its own.

### Shared-fixture dispatch

A run streams: each shared fixture's state is emitted as its `BeforeAll` completes, and suites are dispatched as soon as *their* transitive dependencies are ready, with per-suite state files.
`spec`, `summary` and `watch` run the same way.
A `bench` run waits for its first window of shared fixtures instead, and for every package to compile, before it dispatches: benchmarks run one at a time, each reading its own state file of exactly the fixtures it declares.

### Failure semantics

- A panic inside a fixture *setup* hook is recovered and converted to an error — it fails setup, it does not crash the process.
  Teardown-side panics are contained the same way: a panicking `AfterAll` is recovered and reported as `<fixture>.AfterAll panicked`, a panicking `Dehydrate` as `dehydrate panicked` — both become teardown failures, and teardown of the remaining fixtures continues.
- Shared-fixture setup failure fails the suites that read the fixture, and the run with exit code 1. Each of those suites is booked into the event stream as failed with `<suite> never ran: shared fixture <fixture> did not come up`; the failure itself is booked as the failed package `shared fixtures`, carrying what the fixture reported (`<fixture>.BeforeAll failed after N attempt(s): …`). Nothing else is affected: fixtures that came up stay up, the suites that read only those run, and so does every suite that reads none. A `bench` run gives up the benchmarks that read the failed fixture the same way — whether it failed in the up-front window or when a later slot asked for it — and runs the rest. `prepare` exits 2: bringing the fixtures up is all it does.
  In-test-process package-fixture setup failure is a `t.Fatalf` (exit 1).
- Fixture teardown failure flips an otherwise passing run to a failure. For a shared fixture it is booked as the failed package `shared fixtures`, carrying the `<fixture>.AfterAll failed: …` lines. For a package fixture it fails the package, not a test: after the tests the binary prints the `<fixture>.AfterAll failed: …` lines and `FAIL: fixture teardown failed`, and exits 1.
- Barrier-time failures — an early teardown or a tail-phase start — fail the run through the same aggregation as run-end teardown failures; the terminal teardown still runs and owns the remainder.

### Config markers

- Config marker methods are invoked on a zero-value instance (`(&MyFixture{}).FixtureConfig()`) — the config cannot depend on fixture state.
- The marker must match the fixture kind: `FixtureConfig()` on a shared fixture (or `SharedFixtureConfig()` on a package fixture) is a generation error.

### Generic fixtures

Generic fixture types instantiated with type arguments are supported; the generator mangles type arguments into the fixture identifier so each instantiation is an independent DAG node.

## Migrating from TestMain

### Before

```go
var suite *testhelper.Suite

func TestMain(m *testing.M) {
    // ... 200 lines of setup with os.Exit(1) error handling ...
    suite = &testhelper.Suite{ /* ... */ }
    code := m.Run()
    os.Exit(code)
}

type BatchTestSuite struct{}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    suite.POST(t.T(), "/v1/batch", nil) // package global
}
```

### After

```go
type E2ESetupFixture struct {
    testhelper.Suite
}

func (s *E2ESetupFixture) BeforeAll(ctx context.Context) error {
    // Same setup, but errors are returned and cleanup is automatic
    return nil
}

type BatchTestSuite struct {
    Fixture *E2ESetupFixture
}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    resp := s.Fixture.POST(t.T(), "/v1/batch", payload, s.Fixture.SKKeyFull)
    _ = resp
}
```

Key improvements:
- `BeforeAll` returns `error` — no `os.Exit` needed
- `AfterAll` handles teardown (no manual defer chains)
- No package-level singletons
- Type-safe field access via named fields

### Keeping a TestMain

A `TestMain` that does process-wide work besides fixture setup can stay. It hands the run to `gotestruntime.Main`, which runs the tests and then tears the fixtures down:

```go
func TestMain(m *testing.M) {
    slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
    os.Exit(gotestruntime.Main(m))
}
```

`examples/repository` declares one.

A library that takes `m` to run the tests itself gets the wrapper, which tears the fixtures down before the library's own check runs:

```go
func TestMain(m *testing.M) { goleak.VerifyTestMain(gotestruntime.M(m)) }
```

Under gotest a `TestMain` runs once per suite process, as package fixtures do: the runner starts each suite in a process of its own. Setup placed in a `TestMain` therefore runs for every suite, including those that do not need it, with no timeout, retry, report or teardown on interrupt. Only shared fixtures run once per run.

## Resource Management

Test resources that need setup and teardown (database connections, caches, services) should be stored as suite fields and managed through lifecycle hooks.
Avoid using `defer` or `t.T().Cleanup()` in test methods — these bypass the suite lifecycle.

```go
type OrderTestSuite struct {
    Fixture *E2ESetupFixture
    cache   *OrderCache
    svc     *OrderService
}

func (s *OrderTestSuite) BeforeEach(t *gotest.T) {
    s.cache = NewOrderCache(s.Fixture.Pool, 5*time.Minute)
    s.svc = NewOrderService(s.Fixture.Pool, s.cache)
}

func (s *OrderTestSuite) AfterEach(t *gotest.T) {
    s.cache.Shutdown()
}

func (s *OrderTestSuite) TestCreate(t *gotest.T) {
    t.When("valid order data", func(w *gotest.T) {
        w.It("persists the order", func(it *gotest.T) {
            // s.svc and s.cache are ready — no setup/cleanup in test code
            err := s.svc.Create(context.Background(), order)
            gotest.NoError(it, err)
        })
    })
}
```

When different test methods need fundamentally different service configurations, split them into separate suites — each with its own `BeforeEach`/`AfterEach`.
This keeps resource management declarative and co-located.
