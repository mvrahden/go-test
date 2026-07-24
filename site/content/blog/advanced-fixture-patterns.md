---
title: "Advanced Go Test Fixtures: Composition, Containers, Config"
date: 2026-07-20
description: "Go test fixture patterns at scale: composition, container-backed fixtures, per-test isolation, thread-safe state, and per-environment configuration."
tags: ["Deep Dive"]
keywords: ["go test fixtures", "testcontainers go", "go fixture composition", "go test per-test isolation"]
cta_text: "Compose your first fixture DAG."
toc: true
---

A single fixture gets you started: start a thing, stop a thing, point your tests at it. Real projects need more. Fixtures that compose into dependency graphs, containers that survive flaky startup, per-test isolation that keeps parallel tests from stepping on each other, and configuration that adapts to each environment. These are the patterns that emerge when fixtures grow beyond a single struct with two hooks.

If you are new to gotest fixtures, [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}) covers the fundamentals: struct types with lifecycle hooks, automatic dependency resolution, DAG-based ordering. [Sharing Test Fixtures Across Go Packages]({{< ref "/blog/shared-fixtures" >}}) covers cross-package sharing through JSON state transfer and subprocess lifecycle management. This post assumes you have built your first fixture.

## Fixture composition

When one fixture depends on another, you express it as a pointer field:

```go {title="fixtures_test.go"}
type ServiceFixture struct {
    DB    *DatabaseFixture
    Cache *CacheFixture
    svc   *MyService
}

func (f *ServiceFixture) BeforeAll(ctx context.Context) error {
    f.svc = NewMyService(f.DB.Pool, f.Cache.Client)
    return nil
}
```

The generator sees the pointer fields, resolves the dependency graph, and starts `DatabaseFixture` and `CacheFixture` before `ServiceFixture`. Teardown runs in reverse order.

The key insight: `f.DB` and `f.Cache` are fully initialized by the time `ServiceFixture.BeforeAll` runs. You do not need to check for nil, do not need to pass dependencies manually, do not need to worry about ordering. The DAG handles it.

This scales to deeper graphs. Consider a three-level composition:

```diagram
DatabaseFixture ──┐
                   ├── ServiceFixture
CacheFixture ─────┘        │
                      ┌────┘
                      ├── APITestSuite
                      └── WorkerTestSuite
```

Multiple suites can reference the same fixture. It is initialized once per package, shared across suites. `APITestSuite` and `WorkerTestSuite` both get the same `ServiceFixture` instance, which in turn shares the same `DatabaseFixture` and `CacheFixture`. No duplication, no coordination code.

## Embedding vs. referencing

There are two ways to use a fixture in a suite. The right choice depends on how tests interact with the fixture.

### Reference (pointer field)

```go {title="order_test.go"}
type OrderTestSuite struct {
    DB *DatabaseFixture
}

func (s *OrderTestSuite) TestCreateOrder(t *gotest.T) {
    // Access through the fixture field
    row := s.DB.Pool.QueryRow(ctx, "SELECT ...")
    // ...
}
```

### Embedding

```go {title="pricing_test.go"}
type PricingTestSuite struct {
    *CatalogFixture
}

func (s *PricingTestSuite) TestDiscountCalculation(t *gotest.T) {
    // CatalogFixture's methods are promoted to the suite
    product := s.GetProduct("SKU-001")
    prices := s.ListPrices(product.ID)
    // ...
}
```

Embedding promotes the fixture's exported methods to the suite. This is useful when the fixture provides a domain-specific API (catalog operations, user factories) that tests call frequently. Referencing is better when you need explicit access to infrastructure like connection pools or containers, where the indirection through a named field makes the code clearer.

The generator treats both forms identically for dependency resolution. The difference is purely ergonomic.

## Container-backed fixtures

For fixtures that manage Docker containers (databases, message brokers, caches), gotest provides a preset configuration:

```go {title="config preset"}
func (f *PostgresFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()
    // Timeout: 5 minutes, Retries: 1, RetryDelay: 5 seconds
}
```

This acknowledges reality: containers take time to start and sometimes fail transiently. The retry with delay handles flaky Docker daemon responses. The 5-minute timeout accommodates cold image pulls.

Here is a complete testcontainers example:

```go {title="postgres_fixture_test.go"}
type PostgresFixture struct {
    container testcontainers.Container
    Pool      *pgxpool.Pool
    DSN       string
}

func (f *PostgresFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()
}

func (f *PostgresFixture) BeforeAll(ctx context.Context) error {
    req := testcontainers.ContainerRequest{
        Image:        "postgres:16-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_DB":       "test",
            "POSTGRES_PASSWORD": "test",
        },
        WaitingFor: wait.ForListeningPort("5432/tcp"),
    }
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    if err != nil {
        return err
    }
    f.container = container

    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "5432/tcp")
    f.DSN = fmt.Sprintf("postgres://postgres:test@%s:%s/test?sslmode=disable", host, port.Port())

    pool, err := pgxpool.New(ctx, f.DSN)
    if err != nil {
        return err
    }
    f.Pool = pool
    return nil
}

func (f *PostgresFixture) AfterAll(ctx context.Context) error {
    f.Pool.Close()
    return f.container.Terminate(ctx)
}
```

`BeforeAll` starts the container, waits for it to accept connections, and creates a connection pool. `AfterAll` closes the pool and terminates the container. The `ContainerFixtureConfig` preset gives the container up to 5 minutes to start and retries once on transient failure. All of this is fixture-internal; suites that reference `PostgresFixture` just see `f.Pool` and `f.DSN`.

## Per-test isolation with BeforeEach/AfterEach

Fixture-level `BeforeEach`/`AfterEach` wraps the suite's own hooks. This is perfect for transaction-per-test isolation:

```go {title="postgres_fixture_test.go"}
func (f *PostgresFixture) BeforeEach(ctx context.Context) error {
    _, err := f.Pool.Exec(ctx, "BEGIN")
    return err
}

func (f *PostgresFixture) AfterEach(ctx context.Context) error {
    _, err := f.Pool.Exec(ctx, "ROLLBACK")
    return err
}
```

Every test in every suite that uses this fixture automatically runs in a transaction that rolls back after the test. The suite does not need to know. Isolation is a fixture concern, not a test concern.

This pattern separates two lifecycle scopes cleanly. The container lifecycle (`BeforeAll`/`AfterAll`) runs once per package. The transaction lifecycle (`BeforeEach`/`AfterEach`) runs once per test. A suite that references `PostgresFixture` gets both: the container is already running when the suite starts, and each test gets a fresh transaction.

## Custom configuration and presets

`FixtureConfig` is composable. Start from a preset and override:

```go {title="slow_service_fixture_test.go"}
func (f *SlowServiceFixture) FixtureConfig() gotest.FixtureConfig {
    cfg := gotest.ContainerFixtureConfig()
    cfg.Timeout = 10 * time.Minute  // Slow image pull
    cfg.Retries = 3                  // Flaky network
    return cfg
}
```

The preset gives you sensible defaults. You override what your environment demands. A fixture pulling a 2 GB ML model image needs a longer timeout. A fixture connecting to an external staging service with spotty DNS needs more retries.

Suites have their own configuration through `SuiteConfig`:

```go {title="integration_test.go"}
func (s *IntegrationTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.IntegrationSuiteConfig()
    // Timeout: 2 minutes, SetupTimeout: 5 minutes
}
```

This is the same composability pattern: start from a preset, override what you need. The presets encode common timeout/retry combinations so that every fixture does not need to reinvent them.

## Thread-safe fixture state

When suites run tests in parallel, fixtures must be thread-safe. The pattern: use exported methods with internal synchronization.

```go {title="store_fixture_test.go"}
type InMemoryStoreFixture struct {
    mu    sync.RWMutex
    items map[string]any
}

func (f *InMemoryStoreFixture) BeforeAll(ctx context.Context) error {
    f.items = make(map[string]any)
    return nil
}

func (f *InMemoryStoreFixture) Put(key string, value any) {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.items[key] = value
}

func (f *InMemoryStoreFixture) Get(key string) (any, bool) {
    f.mu.RLock()
    defer f.mu.RUnlock()
    v, ok := f.items[key]
    return v, ok
}
```

Tests interact with the fixture through its methods, never touching internal state directly. The fixture owns its synchronization. A `RWMutex` allows concurrent reads while serializing writes, which is the common pattern for test state that is read-heavy.

This matters when a suite declares `SuiteConfig{Parallel: true}`. The suite's tests run concurrently, and any shared fixture must handle that. The fixture's API boundary is where thread safety lives.

## Binding shared fixtures to local resources

When a [shared fixture]({{< ref "/blog/shared-fixtures" >}}) (cross-package) provides connection information, a package-local fixture can create the actual connection:

```go {title="infra_fixture_test.go"}
type InfraFixture struct {
    Alpha *AlphaSharedFixture
    conn  *grpc.ClientConn
}

func (f *InfraFixture) BeforeAll(ctx context.Context) error {
    // Alpha is already hydrated with cross-package state
    conn, err := grpc.Dial(f.Alpha.ServiceAddr)
    if err != nil {
        return err
    }
    f.conn = conn
    return nil
}

func (f *InfraFixture) AfterAll(ctx context.Context) error {
    return f.conn.Close()
}
```

The shared fixture provides the connection string, transferred across processes via JSON. The package fixture creates the actual connection, which is a local resource that cannot cross the process boundary. This separation keeps transfer state minimal and local resources properly managed.

The pattern works for any resource that has a serializable address and a non-serializable handle: gRPC connections, HTTP clients with custom transports, database pools, authenticated SDK clients.

## When to extract a fixture

Not every piece of setup needs to be a fixture. Here is a decision guide:

- **One suite uses it:** keep setup in `BeforeAll`/`BeforeEach` on the suite. Extracting adds indirection without benefit.
- **Two suites in the same package use it:** extract to a fixture. The DAG handles initialization order and shared state.
- **Multiple packages need it:** extract to a [shared fixture]({{< ref "/blog/shared-fixtures" >}}). The subprocess lifecycle and JSON transfer handle cross-process coordination.
- **It manages a container or external service:** always extract. You want `FixtureConfig` for timeouts and retries, and you want the container lifecycle decoupled from any single suite.

The threshold is duplication. If you find two suites with the same `BeforeAll` body, that body is a fixture waiting to be extracted. And if a package has only a handful of tests with cheap setup, none of this machinery is warranted — a plain helper function is fine.

## A composed fixture stack

These patterns compose. A realistic integration test setup might look like this:

```go {title="integration_test.go"}
// Fixture: manages a Postgres container with per-test transactions
type PostgresFixture struct { /* ... container-backed + per-test isolation ... */ }

// Fixture: manages a Redis container
type RedisFixture struct { /* ... container-backed ... */ }

// Fixture: composes DB + cache, creates a service layer
type ServiceFixture struct {
    DB    *PostgresFixture   // composition
    Cache *RedisFixture      // composition
    Svc   *OrderService
}

// Suite: embeds the service fixture for ergonomic access
type OrderFlowTestSuite struct {
    *ServiceFixture        // embedding
}

func (s *OrderFlowTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.IntegrationSuiteConfig()  // presets
}

func (s *OrderFlowTestSuite) TestPlaceOrder(t *gotest.T) {
    // s.Svc is available directly (promoted from embedded ServiceFixture)
    // Each test runs in a transaction (PostgresFixture.BeforeEach)
    // Container startup is retried on failure (ContainerFixtureConfig)
    order, err := s.Svc.Place(ctx, cart)
    gotest.NoError(t, err)
    gotest.Equal(t, "confirmed", order.Status)
}
```

The test itself is four lines. The infrastructure, lifecycle, isolation, and retry logic all live in the fixtures. Add another suite that needs the same infrastructure and it references the same fixtures. The DAG prevents duplication.

## Further reading

Each pattern here builds on the fundamentals. If you have not read them yet:

- [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}) covers the progression from global variables to DAG-based fixtures.
- [Shared Fixtures Across Packages]({{< ref "/blog/shared-fixtures" >}}) covers the subprocess model and JSON state transfer for cross-package sharing.
- The [reference docs]({{< ref "/reference" >}}) have the full API for `FixtureConfig`, `SuiteConfig`, and lifecycle hook signatures.
