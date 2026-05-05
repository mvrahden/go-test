# Fixtures

Fixtures replace `TestMain` + package-level singletons with convention-driven setup. Any struct whose name ends in `Fixture` is a package fixture; any struct ending in `SharedFixture` is a cross-package shared fixture.

## Package Fixture (`*Fixture` suffix)

A package fixture runs `BeforeAll` once per package, then injects state into child test suites via named pointer fields.

```go
// fixture_test.go

type E2ESetupFixture struct {
    Pool      *pgxpool.Pool
    ServerURL string
    OrgID     uuid.UUID
}

func (f *E2ESetupFixture) BeforeAll(t *gotest.T) {
    pg, err := testhelper.StartPostgres(t.Context())
    gotest.NoError(t, err)
    f.container = pg.Container
    f.Pool = pg.Pool
    // ... wire API, seed fixtures ...
}

func (f *E2ESetupFixture) AfterAll(t *gotest.T) {
    f.Pool.Close()
    f.container.Terminate(context.Background())
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

- One root `*Fixture` per package (nested child fixtures are allowed).
- The fixture struct must have `BeforeAll(t *gotest.T)`. `AfterAll(t *gotest.T)` is optional.
- `BeforeEach(t *gotest.T)` and `AfterEach(t *gotest.T)` are optional and run around every test case in all child suites.
- TestSuites reference the fixture via a named pointer field (`Fixture *E2ESetupFixture`).
- The code generator owns `TestMain` when a fixture is present. Remove any user-defined `TestMain`.

### Generated test output

```
Test_E2ESetupFixture/BatchTestSuite/TestDispatch        PASS
Test_E2ESetupFixture/KeyTestSuite/TestCreate             PASS
```

Filter with `-run Test_E2ESetupFixture/BatchTestSuite` to run only batch tests.

## Nested Fixtures (Level 2)

Fixtures can reference other fixtures via named pointer fields to form a dependency tree. The root fixture's `BeforeAll` runs first, then each child fixture's `BeforeAll`.

```go
type InfraFixture struct {
    Pool *pgxpool.Pool
}

func (f *InfraFixture) BeforeAll(t *gotest.T) { /* start containers */ }
func (f *InfraFixture) AfterAll(t *gotest.T)  { /* terminate containers */ }

type APIFixture struct {
    Infra     *InfraFixture
    ServerURL string
}

func (f *APIFixture) BeforeAll(t *gotest.T) {
    srv := api.NewServer(":0", api.Dependencies{Pool: f.Infra.Pool})
    f.ServerURL = srv.URL
}

type ReconcilerTestSuite struct { Infra *InfraFixture }  // only needs DB
type BatchTestSuite struct { API *APIFixture }            // needs full API
```

### Generated test output

```
Test_InfraFixture/ReconcilerTestSuite/TestOrphan        PASS
Test_InfraFixture/APIFixture/BatchTestSuite/TestDispatch PASS
```

Filter with `-run Test_InfraFixture/ReconcilerTestSuite` to skip the API stack entirely.

## Shared Fixtures (Level 3, `*SharedFixture` suffix)

Shared fixtures run in a subprocess managed by the `gotest` CLI. They start once per CLI invocation and are shared across all packages. State crosses the process boundary via JSON serialization, with `Hydrate` handling reconstruction of non-serializable resources.

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

- Shared fixture types must live in a **non-internal** package (not under `internal/`). The setup subprocess compiles outside the module tree and cannot import `internal/` paths. Shared fixtures may freely import and depend on `internal/` packages — only the fixture type's own package path is restricted.
- `BeforeAll(ctx context.Context) error` and `AfterAll(ctx context.Context) error` — runs in the subprocess.
- `Hydrate(ctx context.Context) error` and `Dehydrate(ctx context.Context) error` — runs in the test process, optional.
- `BeforeEach`/`AfterEach` are not allowed on shared fixtures.
- Exported fields are serialized as JSON and transferred to the test process via a state file (`GOTEST_SHARED_STATE_FILE`).
- Fields assigned in `Hydrate` (directly, or in receiver methods called from `Hydrate`, one level deep) are **local** — excluded from serialization and reconstructed by `Hydrate`.

### State transfer

In the example above, `ConnStr` and `Port` are transferable (not assigned in `Hydrate`'s call chain). `Pool` is local (assigned in `connect()`, which is called from `Hydrate`).

Convention: in `Hydrate`, assign to local fields. Read transferred fields but do not reassign them — use local variables for any transformation.

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

A standalone suite can reference multiple shared fixtures. Suites without any shared fixture fields coexist in the same package without issue.

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

func (s *E2ESetupFixture) BeforeAll(t *gotest.T) {
    // Same setup, but t.Fatal() works and no os.Exit needed
}

type BatchTestSuite struct {
    Fixture *E2ESetupFixture
}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    resp := s.Fixture.POST(t.T(), "/v1/batch", ..., s.Fixture.SKKeyFull)
}
```

Key improvements:
- `t.Fatal()` works in setup (runs in test context)
- `AfterAll` handles teardown (no manual defer chains)
- No package-level singletons
- Type-safe field access via named fields

## Resource Management

Test resources that need setup and teardown (database connections, caches, services) should be stored as suite fields and managed through lifecycle hooks. Avoid using `defer` or `t.T().Cleanup()` in test methods — these bypass the suite lifecycle.

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

When different test methods need fundamentally different service configurations, split them into separate suites — each with its own `BeforeEach`/`AfterEach`. This keeps resource management declarative and co-located.
