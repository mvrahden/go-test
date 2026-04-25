# Fixtures

Fixtures replace `TestMain` + package-level singletons with directive-driven setup that composes via Go embedding.

## Package Fixture (`//go:test fixture`)

A package fixture runs `BeforeAll` once per package, then injects state into child test suites via pointer embedding.

```go
// fixture_test.go

//go:test fixture
type E2ESetup struct {
    Pool      *pgxpool.Pool
    ServerURL string
    OrgID     uuid.UUID
}

func (s *E2ESetup) BeforeAll(t *gotest.T) {
    pg, err := testhelper.StartPostgres(ctx)
    gotest.NoError(t, err)
    t.T().Cleanup(func() { pg.Container.Terminate(ctx) })
    s.Pool = pg.Pool
    // ... wire API, seed fixtures ...
}

func (s *E2ESetup) AfterAll(t *gotest.T) {
    s.Pool.Close()
}
```

```go
// batch_test.go

type BatchTestSuite struct {
    *E2ESetup // s.Pool, s.ServerURL, s.OrgID available via embedding
}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    // s.Pool is populated by E2ESetup.BeforeAll
}
```

### Rules

- One root `//go:test fixture` per package (nested child fixtures are allowed).
- The fixture struct must have `BeforeAll(t *gotest.T)`. `AfterAll(t *gotest.T)` is optional.
- `BeforeEach(t *gotest.T)` and `AfterEach(t *gotest.T)` are optional and run around every test case in all child suites.
- TestSuites embed the fixture via pointer embedding (`*E2ESetup`).
- The code generator owns `TestMain` when a fixture is present. Remove any user-defined `TestMain`.

### Generated test output

```
Test_E2ESetup/BatchTestSuite/TestDispatch        PASS
Test_E2ESetup/KeyTestSuite/TestCreate             PASS
```

Filter with `-run Test_E2ESetup/BatchTestSuite` to run only batch tests.

## Nested Fixtures (Level 2)

Fixtures can embed other fixtures to form a dependency tree. The root fixture's `BeforeAll` runs first, then each child fixture's `BeforeAll`.

```go
//go:test fixture
type InfraFixture struct {
    Pool *pgxpool.Pool
}

func (f *InfraFixture) BeforeAll(t *gotest.T) { /* start containers */ }
func (f *InfraFixture) AfterAll(t *gotest.T)  { /* terminate containers */ }

//go:test fixture
type APIFixture struct {
    *InfraFixture  // f.Pool available from parent
    ServerURL string
}

func (f *APIFixture) BeforeAll(t *gotest.T) {
    srv := api.NewServer(":0", api.Dependencies{Pool: f.Pool})
    f.ServerURL = srv.URL
}

type ReconcilerTestSuite struct { *InfraFixture }  // only needs DB
type BatchTestSuite struct { *APIFixture }          // needs full API
```

### Generated test output

```
Test_InfraFixture/ReconcilerTestSuite/TestOrphan        PASS
Test_InfraFixture/APIFixture/BatchTestSuite/TestDispatch PASS
```

Filter with `-run Test_InfraFixture/ReconcilerTestSuite` to skip the API stack entirely.

## Shared Fixtures (Level 3, `//go:test shared-fixture`)

Shared fixtures run in a subprocess managed by the `testsuite` CLI. They start once per CLI invocation and are shared across all packages.

```go
// tests/fixtures/postgres.go

//go:test shared-fixture
type PostgresFixture struct {
    DSN       string `gotest:"env=E2E_POSTGRES_DSN"`
    container testcontainers.Container
}

func (f *PostgresFixture) BeforeAll() error {
    c, err := testhelper.StartPostgres(context.Background())
    if err != nil {
        return err
    }
    f.container = c.Container
    f.DSN = c.ConnectionString
    return nil
}

func (f *PostgresFixture) AfterAll() error {
    return f.container.Terminate(context.Background())
}
```

### Rules

- `BeforeAll() error` and `AfterAll() error` — no `*gotest.T` (runs outside test context).
- Exported fields with `gotest:"env=VAR_NAME"` tags transfer state as environment variables.
- `BeforeEach`/`AfterEach` are not allowed on shared fixtures.

### Using shared fixtures in package fixtures

Package fixtures embed shared fixture types. The code generator resolves tagged fields from env vars:

```go
//go:test fixture
type E2ESetup struct {
    *fixtures.PostgresFixture // DSN populated from E2E_POSTGRES_DSN env var
    Pool *pgxpool.Pool
}

func (s *E2ESetup) BeforeAll(t *gotest.T) {
    pool, err := pgxpool.New(ctx, s.DSN) // s.DSN comes from shared fixture
    gotest.NoError(t, err)
    s.Pool = pool
}
```

### CLI flow

```
testsuite ./tests/e2e ./tests/integration -v
```

1. Scan target packages for shared fixtures
2. Generate and start a setup subprocess (calls `BeforeAll`, exports env vars as JSON)
3. Generate test code for each package
4. Run `go test` with shared fixture env vars in the environment
5. Send SIGTERM to the setup subprocess (calls `AfterAll` in reverse order)

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
//go:test fixture
type E2ESetup struct {
    testhelper.Suite
}

func (s *E2ESetup) BeforeAll(t *gotest.T) {
    // Same setup, but t.Fatal() works and no os.Exit needed
}

type BatchTestSuite struct {
    *E2ESetup // s.POST(), s.Pool — via embedding
}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    resp := s.POST(t.T(), "/v1/batch", ..., s.SKKeyFull)
}
```

Key improvements:
- `t.Fatal()` works in setup (runs in test context)
- `t.T().Cleanup()` for teardown (no manual defer chains)
- No package-level singletons
- Type-safe field access via embedding
