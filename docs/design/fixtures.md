# Fixtures

Fixtures replace `TestMain` + package-level singletons with convention-driven setup.
Any struct whose name ends in `Fixture` is a package fixture; any struct ending in `SharedFixture` is a cross-package shared fixture.

## Package Fixture (`*Fixture` suffix)

A package fixture runs `BeforeAll` once per package, then injects state into child test suites via named pointer fields.

```go
// fixture_test.go

type E2ESetupFixture struct {
    Pool      *pgxpool.Pool
    ServerURL string
    OrgID     uuid.UUID
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
- Fixtures do not use `TestMain`. User-defined `TestMain` functions coexist with fixtures without conflict.

### Generated test output

Fixture setup runs automatically before the first fixture-bound test — fixture names do not appear in test paths.
Suites bound to a fixture produce the same test names as standalone suites:

```
TestBatchTestSuite/TestDispatch        PASS
TestKeyTestSuite/TestCreate            PASS
```

Filter with `-run TestBatchTestSuite` to run only batch tests.

## Fixture Dependencies (Level 2)

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

## Shared Fixtures (Level 3, `*SharedFixture` suffix)

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
- SharedFixtures cannot depend on PackageFixtures (they run in different processes).

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

## Migrating from TestMain

### Before

```go
var suite *testhelper.Suite

func TestMain(m *testing.M) {
    // ... 200 lines of setup with os.Exit(1) error handling ...
    suite = &testhelper.Suite{...}
    code := m.Run()
    os.Exit(code)
}

type BatchTestSuite struct{}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    suite.POST(t.T(), ...) // package global
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
    resp := s.Fixture.POST(t.T(), "/v1/batch", ..., s.Fixture.SKKeyFull)
}
```

Key improvements:
- `BeforeAll` returns `error` — no `os.Exit` needed
- `AfterAll` handles teardown (no manual defer chains)
- No package-level singletons
- Type-safe field access via named fields

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
