---
title: "Sharing Test Fixtures Across Go Packages"
date: 2026-07-19
description: "go test runs each package in a separate OS process. How do you share a database container across packages? gotest's SharedFixture model solves it."
tags: ["Deep Dive"]
keywords: ["share database across go test packages", "go testmain fixture", "go shared test fixtures", "go test process boundary"]
cta_text: "Try shared fixtures in your next project."
toc: true
---

`go test` compiles each package into a separate test binary and runs it as a separate OS process. That design buys you isolation, and it makes one thing structurally difficult: sharing a fixture *across* packages. A Postgres container that `pkg/user`, `pkg/order`, and `pkg/billing` all need. A Redis instance that three packages query. A schema migration that should run once for the entire test run, not once per package. Sharing any of these means crossing process boundaries.

None of the mainstream Go testing frameworks ships a built-in answer for it. gotest does, through a mechanism called shared fixtures.

A [previous post]({{< ref "/blog/test-fixtures-in-go" >}}) covered fixture patterns within a single package: global variables, helper functions, `sync.Once` wrappers, and gotest's DAG-based package fixtures. All of those work within one test binary, one process. This post is about the harder problem: crossing that boundary.

## Why this is hard

The process-per-package model is not a quirk of the implementation — it is a deliberate design choice that gives you process-level isolation between packages.

The consequence: there is no shared memory between `pkg/user` and `pkg/order`. They are different binaries, different processes, different address spaces. A `*sql.DB` created in one package cannot be passed to another. A `sync.Once` in one process cannot coordinate with a `sync.Once` in another.

This means every cross-package fixture approach in the stdlib has the same fundamental problem:

- **`TestMain`** runs once per package, not once per test run. Five packages that need Postgres means five separate setup attempts.
- **`sync.Once`** is a per-process primitive. It cannot cross process boundaries.
- **Helper packages** like `testutil.GetDB()` get imported by each package independently. Each one initializes its own connection in its own process.

## What teams do today

Without a built-in solution, most teams fall back to one of two patterns:

### External orchestration

A Makefile, docker-compose file, or CI script starts the database before `go test` runs. The DSN is passed through an environment variable. Each package's `TestMain` connects independently.

```make {title="Makefile"}
test:
	docker compose up -d postgres
	sleep 3
	TEST_DB_URL=postgres://localhost:5432/testdb go test ./...
	docker compose down
```

This works. The database is shared. But the fixture lifecycle lives outside of Go, in shell scripts and YAML files. The test code cannot express "I need a Postgres instance" as a dependency; it can only assume one already exists. And the `sleep 3` is a reminder that coordination across processes is fundamentally a timing problem when you solve it with scripts.

### Per-package duplication

Each package starts its own container. This is simple and isolated, but expensive. If five packages each start a Postgres container, you're spending 15–30 seconds on container startup alone. In CI, this adds up.

```go {title="pkg/user/testmain_test.go"}
func TestMain(m *testing.M) {
    container := startPostgres()  // 3-6 seconds
    defer container.Stop()
    os.Exit(m.Run())
}
```

```go {title="pkg/order/testmain_test.go"}
func TestMain(m *testing.M) {
    container := startPostgres()  // same 3-6 seconds, again
    defer container.Stop()
    os.Exit(m.Run())
}
```

Both approaches have the same root cause: Go's process-per-package model has no built-in mechanism for cross-process resource sharing.

## Shared fixtures: the model

gotest's answer is **shared fixtures**: structs whose name ends in `SharedFixture`. A shared fixture runs in its own subprocess, once for the entire test run. Its state is serialized as JSON and transferred to every test package that needs it.

The lifecycle looks like this:

```diagram
1. gotest starts a fixture subprocess
2. SharedFixture.BeforeAll runs (start container, run migrations)
3. Exported fields are serialized to JSON (the "transfer state")
4. For each test package that needs this fixture:
   a. Transfer state is deserialized into a new instance
   b. SharedFixture.Hydrate runs (open connections from transfer state)
   c. Tests run
   d. SharedFixture.Dehydrate runs (close connections)
5. All test packages finish
6. SharedFixture.AfterAll runs (stop container, cleanup)
```

The key insight is the split between what can cross a process boundary (data) and what cannot (connections, file handles, goroutines). The fixture struct's **exported fields** are the transfer state: they get serialized to JSON and sent to each test process. The **unexported fields** hold local resources that are reconstructed by `Hydrate` in each process.

## A concrete example

Consider a Postgres container that multiple packages need:

```go {title="testinfra/fixtures.go"}
package testinfra

import (
    "context"
    "database/sql"

    "github.com/mvrahden/go-test/pkg/gotest"
)

type PostgresSharedFixture struct {
    DSN  string   // exported: serialized to JSON, transferred to test packages
    conn *sql.DB  // unexported: local to each process, created by Hydrate
}

func (f *PostgresSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()  // 5m timeout, 1 retry, 5s delay
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error {
    // Start a container, run migrations, seed data.
    // This runs once for the entire test run.
    container, err := startPostgresContainer(ctx)
    if err != nil {
        return err
    }
    f.DSN = container.DSN()

    f.conn, err = sql.Open("postgres", f.DSN)
    if err != nil {
        return err
    }
    _, err = f.conn.ExecContext(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT)")
    return err
}

func (f *PostgresSharedFixture) AfterAll(ctx context.Context) error {
    if f.conn != nil {
        f.conn.Close()
    }
    // Stop the container.
    return nil
}

func (f *PostgresSharedFixture) Hydrate(ctx context.Context) error {
    // Called in each test package's process.
    // f.DSN is already populated from the JSON transfer.
    var err error
    f.conn, err = sql.Open("postgres", f.DSN)
    return err
}

func (f *PostgresSharedFixture) Dehydrate(ctx context.Context) error {
    // Called when a test package's process is done.
    if f.conn != nil {
        return f.conn.Close()
    }
    return nil
}

// Conn returns the database connection for test code to use.
func (f *PostgresSharedFixture) Conn() *sql.DB {
    return f.conn
}
```

The lifecycle methods map to distinct moments:

- **`BeforeAll`** runs once, in the fixture subprocess. Start the container, run migrations, set the DSN. The expensive work happens here, exactly once.
- **`Hydrate`** runs in each test package's process, after the exported fields (`DSN`) have been deserialized from JSON. It reconstructs the local resources: opens a database connection from the DSN. This is cheap — the container is already running.
- **`Dehydrate`** runs when a test package's process finishes. It cleans up local resources: closes the connection. The container stays running for the next package.
- **`AfterAll`** runs once, after all test packages are done. Stop the container, delete temp files, release any remaining resources.

For the ordering and cleanup guarantees behind hooks like these — what runs when, and what still runs after a failure — see [Go Test Lifecycle]({{< ref "/blog/go-test-lifecycle" >}}).

## Consuming shared fixtures from test suites

A suite declares its dependency on a shared fixture the same way it declares any fixture dependency: a pointer field.

```go {title="pkg/user/suite_test.go"}
package user

import (
    "github.com/mvrahden/go-test/pkg/gotest"
    "yourproject/testinfra"
)

type UserRepositoryTestSuite struct {
    Postgres *testinfra.PostgresSharedFixture
    repo     *userRepository
}

func (s *UserRepositoryTestSuite) BeforeEach(t *gotest.T) {
    s.repo = newUserRepository(s.Postgres.Conn())
}

func (s *UserRepositoryTestSuite) TestCreateUser(t *gotest.T) {
    t.When("the input is valid", func(w *gotest.T) {
        err := s.repo.Create(User{ID: "1", Email: "alice@example.com"})

        w.It("succeeds", func(it *gotest.T) {
            gotest.NoError(it, err)
        })
    })
}
```

```go {title="pkg/order/suite_test.go"}
package order

import (
    "github.com/mvrahden/go-test/pkg/gotest"
    "yourproject/testinfra"
)

type OrderRepositoryTestSuite struct {
    Postgres *testinfra.PostgresSharedFixture
    repo     *orderRepository
}

func (s *OrderRepositoryTestSuite) BeforeEach(t *gotest.T) {
    s.repo = newOrderRepository(s.Postgres.Conn())
}

func (s *OrderRepositoryTestSuite) TestPlaceOrder(t *gotest.T) {
    // Uses the same Postgres container as UserRepositoryTestSuite
    // but in a different OS process
}
```

Both suites reference `*testinfra.PostgresSharedFixture`. gotest sees the pointer to a `SharedFixture`-suffixed type and wires it up. The container starts once. Both packages get their own database connection, hydrated from the same DSN. When both packages are done, the container stops.

## Transfer fields vs. local fields

The distinction between exported and unexported fields is the core of the shared fixture model. It maps directly to what can and cannot cross a process boundary:

- **Exported fields** (`DSN string`, `Port int`, `Token string`) are serialized to JSON. They must be JSON-marshalable. These are the connection coordinates: the information another process needs to reach the same resource.
- **Unexported fields** (`conn *sql.DB`, `handle *os.File`) are local to each process. They are assigned by `Hydrate`, cleaned up by `Dehydrate`. They represent the live connection, not the address.

This split is explicit and enforced by Go's export rules. You cannot accidentally serialize a `*sql.DB` because it is unexported and `encoding/json` ignores unexported fields.

> Not every shared fixture needs `Hydrate` and `Dehydrate`. If the fixture's exported fields are sufficient for test code to work with (a port number, a URL, a token), you can skip them. `Hydrate`/`Dehydrate` are for resources that need to be opened and closed in each process.

## Shared fixture dependencies

Shared fixtures can depend on other shared fixtures, forming a DAG just like package fixtures. A schema migration fixture that depends on a Postgres container:

```go {title="testinfra/fixtures.go"}
type SchemaSharedFixture struct {
    Postgres *PostgresSharedFixture  // depends on Postgres being up
    Version  string
}

func (f *SchemaSharedFixture) BeforeAll(ctx context.Context) error {
    // f.Postgres.conn is live (Postgres.BeforeAll already ran)
    _, err := f.Postgres.Conn().ExecContext(ctx,
        "CREATE TABLE IF NOT EXISTS orders (id TEXT, user_id TEXT, total NUMERIC)")
    if err != nil {
        return err
    }
    f.Version = "v2"
    return nil
}

func (f *SchemaSharedFixture) AfterAll(_ context.Context) error {
    return nil
}
```

gotest resolves these dependencies in topological order. `PostgresSharedFixture.BeforeAll` runs first. When it completes, `SchemaSharedFixture.BeforeAll` runs. Independent fixtures (no dependency between them) set up concurrently. Teardown runs in reverse order. Cyclic dependencies are rejected at resolution time with a clear error message.

Suites that depend on `SchemaSharedFixture` automatically get `PostgresSharedFixture` too — transitive dependencies are included. A suite doesn't need to list every fixture in the chain; the pointer to the leaf fixture is enough. For more composition patterns — embedding vs. referencing, and binding shared fixtures to package-local resources — see [Advanced Go Test Fixtures]({{< ref "/blog/advanced-fixture-patterns" >}}).

## Dispatch timing

Suites are dispatched as soon as their specific shared fixture dependencies are ready. They do not wait for unrelated fixtures. If suite A needs Postgres and suite B needs Redis, A starts running as soon as Postgres is ready, even if Redis is still starting.

```diagram
PostgresSharedFixture.BeforeAll ─── ready ──> pkg/user tests start
                                                    pkg/order tests start
RedisSharedFixture.BeforeAll ────── ready ──> pkg/cache tests start
```

This means the wall-clock cost of fixture setup is the longest single fixture, not the sum of all fixtures. If Postgres takes 4 seconds and Redis takes 2 seconds, the total setup overhead is 4 seconds, not 6.

## Configuration and timeouts

Shared fixtures support the same configuration pattern as package fixtures, through a `SharedFixtureConfig()` marker method:

```go
func (f *PostgresSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()  // 5m timeout, 1 retry, 5s delay
}
```

`ContainerFixtureConfig()` is a preset that gives a 5-minute timeout with one retry and a 5-second retry delay — appropriate for infrastructure that might need time to pull an image or start a process. You can also specify values directly:

```go
func (f *PostgresSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
    return gotest.FixtureConfig{
        Timeout: 2 * time.Minute,
        Retries: 2,
    }
}
```

There is also a project-level timeout, `--setup-timeout`, which sets a wall-clock budget for the entire shared fixture setup phase. The two timeouts run concurrently: the per-fixture timeout governs individual fixtures, and the project-level timeout governs the total. Whichever fires first wins.

## When to use shared fixtures vs. package fixtures

The decision is about scope and cost:

- **Package fixtures** (`*Fixture` suffix) are scoped to a single package. They are simpler: no JSON serialization, no `Hydrate`/`Dehydrate`, no subprocess. Use them when the fixture is only needed by suites in one package, or when the setup cost is low enough that duplicating it across packages is acceptable.
- **Shared fixtures** (`*SharedFixture` suffix) are scoped to the entire test run. Use them when the fixture is expensive (containers, external services, large seed datasets) and needed by suites across multiple packages.

If in doubt, start with package fixtures. Promote to shared fixtures when the per-package setup time becomes a problem. The struct pattern is similar enough that the migration is mechanical: rename the suffix, add `Hydrate`/`Dehydrate` for non-serializable fields, and move the struct to a shared package.

## What this replaces

Shared fixtures replace the external orchestration layer that most Go projects use today: the Makefile that starts containers, the docker-compose file that seeds databases, the CI script that waits for health checks. The fixture lifecycle moves into Go, where it is type-checked, version-controlled, and visible in the test code itself.

The test code can now say "I need a Postgres instance with this schema" as a typed dependency, not as an assumption about what `make test` did before `go test` ran. If the fixture fails to start, the error appears in the test output with a file and line number. If the fixture is slow, `ContainerFixtureConfig` gives it retries and a timeout. If the fixture depends on another fixture, the dependency is a pointer field that the generator resolves automatically.

This is the kind of problem that, once you see the solution, feels obvious. But it requires crossing a conceptual boundary: the fixture lifecycle must span multiple OS processes, which means serialization, subprocesses, and careful lifecycle orchestration. That is what the `SharedFixture` model provides.

## Further reading

For the single-package fundamentals this model builds on, start with [Test Fixtures in Go]({{< ref "/blog/test-fixtures-in-go" >}}). For composition patterns, container-backed fixtures, and per-test isolation, continue with [Advanced Go Test Fixtures]({{< ref "/blog/advanced-fixture-patterns" >}}). And for the exact ordering and cleanup guarantees of every hook, see [Go Test Lifecycle]({{< ref "/blog/go-test-lifecycle" >}}).
