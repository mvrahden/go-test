# gotest

<p align="center">
  <img src="static/gopher.png" alt="gotest gopher" width="360" />
</p>

[![CI](https://github.com/mvrahden/go-test/actions/workflows/test.yml/badge.svg)](https://github.com/mvrahden/go-test/actions/workflows/test.yml)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Go tests that write themselves, organize themselves, and explain themselves.

`gotest` closes the gap between `func TestX(t *testing.T)` and a well-organized test suite through code generation.
You write structs, name them well, and the tool handles the rest.
No runtime dependencies.
No reflection.
No lock-in.
Just standard Go tests with lifecycle management and structured organization.

## Why gotest?

Go's `testing` package gives you `func TestX(t *testing.T)` and nothing more.
Setup/teardown logic is copy-pasted or buried in `TestMain`.
As test suites grow, organization becomes a discipline problem rather than a tooling one.

**testify/suite** solves organization but adds runtime reflection, interface dispatch, and a `suite.Run(t, new(MySuite))` ceremony in every file.
Test output is standard — but the mechanism behind it isn't.

**gotest** takes a different approach: you write structs with naming conventions, and a code generator produces the `func Test*` wrappers, lifecycle wiring, and `t.Run` nesting that you'd write by hand.
The generated code is deleted after tests run.
What remains is standard `go test` output, zero runtime dependencies, and no lock-in — `gotest clean` removes all traces, and your test structs still compile.

## Install

```bash
go install github.com/mvrahden/go-test/cmd/gotest@latest
```

## 30-Second Example

Write a test suite struct:

```go
// user_service_suite_test.go
package user

import "github.com/mvrahden/go-test/pkg/gotest"

type UserServiceTestSuite struct {
    svc *UserService
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
    s.svc = NewUserService()
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.It("creates a user with valid input", func(it *gotest.T) {
        err := s.svc.Create("alice@example.com")
        gotest.NoError(it, err)
    })

    t.When("email already exists", func(w *gotest.T) {
        w.It("returns ErrDuplicate", func(it *gotest.T) {
            s.svc.Create("alice@example.com")
            err := s.svc.Create("alice@example.com")
            gotest.ErrorIs(it, err, ErrDuplicate)
        })
    })
}
```

Run:

```bash
gotest ./... -v
```

Output is standard `go test` output:

```
=== RUN   TestUserServiceTestSuite
=== RUN   TestUserServiceTestSuite/TestCreate
=== RUN   TestUserServiceTestSuite/TestCreate/creates_a_user_with_valid_input
=== RUN   TestUserServiceTestSuite/TestCreate/email_already_exists
=== RUN   TestUserServiceTestSuite/TestCreate/email_already_exists/returns_ErrDuplicate
--- PASS: TestUserServiceTestSuite (0.01s)
```

No generated code leaks into your workflow.
`gotest` generates it before tests run and cleans it up after.

## Features

### Lifecycle Hooks

Suite hooks accept either `*gotest.T` or `*testing.T` — you choose per method:

```go
func (s *MySuite) BeforeAll(t *gotest.T)  {} // once before all tests
func (s *MySuite) AfterAll(t *gotest.T)   {} // once after all tests
func (s *MySuite) BeforeEach(t *gotest.T) {} // before each test method
func (s *MySuite) AfterEach(t *gotest.T)  {} // after each test method
```

`*gotest.T` exposes `t.Context()` (mirrors Go 1.24's `testing.T.Context()`), plus the full DSL (`t.It()`, `t.When()`, `t.MatchSnapshot()`).
Use `*testing.T` for plain stdlib tests — the functional assertions (`gotest.Equal(t, ...)`) still work with either type.
You can mix freely within a single suite:

```go
func (s *MySuite) BeforeEach(t *testing.T) {} // stdlib is fine here
func (s *MySuite) TestPlain(t *testing.T)  {} // no gotest import needed
func (s *MySuite) TestRich(t *gotest.T)    {} // full DSL available
```

**Resource management through suite fields.**
Resources that need setup and teardown (database pools, caches, services) should be stored as suite fields and managed through `BeforeEach`/`AfterEach`.
Avoid using `defer` or `t.T().Cleanup()` in test methods — these bypass the suite lifecycle and scatter resource management across test code:

```go
type AuthServiceTestSuite struct {
    Postgres *fixtures.PostgresSharedFixture
    pool     *pgxpool.Pool
    cache    *OrgConfigCache
    svc      *AuthService
}

func (s *AuthServiceTestSuite) BeforeEach(t *gotest.T) {
    s.pool = s.Postgres.NewPool(t)
    s.cache = NewOrgConfigCache(s.pool, 5*time.Minute)
    s.svc = NewAuthService(s.pool, s.cache)
}

func (s *AuthServiceTestSuite) AfterEach(t *gotest.T) {
    s.cache.Shutdown()
}

func (s *AuthServiceTestSuite) TestPermissions(t *gotest.T) {
    t.When("user has admin role", func(w *gotest.T) {
        w.It("allows write access", func(it *gotest.T) {
            // s.pool and s.svc are ready — no setup/cleanup here
            allowed, err := s.svc.Check(ctx, orgID, "write")
            gotest.NoError(it, err)
            gotest.True(it, allowed)
        })
    })
}
```

When different test methods need fundamentally different service configurations, split them into separate suites — each with its own `BeforeEach`/`AfterEach`.
This keeps resource management declarative and predictable.

Fixture hooks receive `context.Context` and return `error` — the generated wrapper reports failures with automatic attribution:

```go
func (f *MyFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *MyFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *MyFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *MyFixture) AfterEach(ctx context.Context) error  { return nil }
```

Setup hooks (`BeforeAll`, `BeforeEach`) receive `t.Context()` — cancelled when the test ends, carries the test deadline.
Cleanup hooks (`AfterAll`, `AfterEach`) receive `context.Background()` — cleanup must proceed even after the test context is cancelled.
Requires Go 1.24+.

All hooks are optional.
`AfterAll` runs via `t.Cleanup` (LIFO).
`AfterEach` is deferred, so it runs even on `t.Fatal()`.

### Fixtures

Fixtures replace `TestMain` + package-level singletons with convention-driven setup.
Any struct ending in `Fixture` is a package fixture; ending in `SharedFixture` is a cross-package shared fixture.

```go
// fixture_test.go

type E2ESetupFixture struct {
    Pool      *pgxpool.Pool
    ServerURL string
}

func (f *E2ESetupFixture) BeforeAll(ctx context.Context) error {
    pg, err := testhelper.StartPostgres(ctx)
    if err != nil {
        return fmt.Errorf("start postgres: %w", err)
    }
    f.Pool = pg.Pool
    return nil
}

func (f *E2ESetupFixture) AfterAll(ctx context.Context) error {
    f.Pool.Close()
    return nil
}
```

Fixture hooks return `error` — the generated wrapper handles reporting with automatic attribution (e.g., `E2ESetupFixture.BeforeAll failed: start postgres: connection refused`).

Test suites reference the fixture via a named pointer field:

```go
type BatchTestSuite struct {
    Fixture *E2ESetupFixture
}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    // s.Fixture.Pool is populated by E2ESetupFixture.BeforeAll
}
```

Fixtures support the same lifecycle hooks as suites.
`BeforeAll`/`AfterAll` run once around all suites bound to the fixture.
`BeforeEach`/`AfterEach` wrap every individual test case, running outside the suite's own hooks:

```
Fixture.BeforeEach
  Suite.BeforeEach
    TestCase
  Suite.AfterEach
Fixture.AfterEach
```

All four hooks are optional (only `BeforeAll` is required).

Fixtures can nest — a root fixture's hooks run first, wrapping the child's:

```go
type InfraFixture struct { Pool *pgxpool.Pool }

type APIFixture struct {
    Infra     *InfraFixture
    ServerURL string
}
```

```
InfraFixture.BeforeEach
  APIFixture.BeforeEach
    Suite.BeforeEach
      TestCase
    Suite.AfterEach
  APIFixture.AfterEach
InfraFixture.AfterEach
```

Output nests naturally: `Test_InfraFixture/APIFixture/BatchTestSuite/TestDispatch`.

For cross-package shared state (e.g. a database container shared across integration test packages), use `*SharedFixture` suffix — see [docs/fixtures.md](docs/fixtures.md) for the full reference.

### Configuration

Every fixture and suite runs with sensible defaults — 2-minute fixture timeout, 30-second per-test timeout.
Override with optional marker methods:

```go
func (f *InfraFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.FixtureConfig{
        Timeout:    5 * time.Minute,
        Retries:    1,
        RetryDelay: 5 * time.Second,
    }
}

func (f *PostgresSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig()
}

func (s *BatchTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.SuiteConfig{
        Timeout:  1 * time.Minute,
        FailFast: true,
    }
}
```

Only non-zero fields override.
Use negative duration to explicitly disable a timeout (`Timeout: -1`).

Preset constructors for common scenarios:

| Preset | Timeout | Retries | Use case |
|--------|---------|---------|----------|
| `DefaultFixtureConfig()` | 2 min | 0 | Standard fixtures |
| `ContainerFixtureConfig()` | 5 min | 1 | Testcontainers, image pulls |
| `DefaultSuiteConfig()` | 30 sec | 0 | Unit/integration tests |
| `IntegrationSuiteConfig()` | 2 min | 0 | Heavier integration tests |

### Focus and Exclude

```go
type F_UserServiceTestSuite struct { ... }  // F_ prefix: only this suite runs
type X_BrokenTestSuite struct { ... }       // X_ prefix: this suite is skipped

func (s *MySuite) F_TestCreate(t *gotest.T) {} // focus a single test
func (s *MySuite) X_TestFlaky(t *gotest.T)  {} // exclude a single test
```

Use `--ci` in CI to fail the build if any `F_` prefix slipped through:

```bash
gotest --ci ./... -v -race
```

### SuiteGuard

Skip a suite at runtime based on environment conditions:

```go
func (s *IntegrationTestSuite) SuiteGuard() string {
    if os.Getenv("DATABASE_URL") == "" {
        return "DATABASE_URL not set"
    }
    return "" // empty = run
}
```

Returns a non-empty reason to skip the entire suite.
Unlike `X_` (static exclude), `SuiteGuard` makes the decision at runtime — useful for integration tests that need external services.

### BDD Vocabulary

```go
func (s *Suite) TestCreate(t *gotest.T) {
    t.When("input is valid", func(w *gotest.T) {
        w.It("creates the record", func(it *gotest.T) {
            // ...
        })
    })
}
```

`When` groups context.
`It` specifies behavior.
Both map to `t.Run` under the hood.

### Parallel Tests

**Suite-level parallelism** is automatic — the `gotest` runner executes each suite's test binary as a separate subprocess, giving full process isolation with zero shared state between suites.

**Method-level parallelism** is opt-in via `SuiteConfig{Parallel: true}`.
When enabled, each test method runs concurrently.
Because the suite struct is shared, parallel methods can't safely mutate it — instead, `BeforeEach` returns a per-test context struct that each method receives as a second argument:

```go
type MethodParallelCtx struct {
    Value int64
}

type MyTestSuite struct{}

func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.SuiteConfig{Parallel: true}
}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *MethodParallelCtx {
    return &MethodParallelCtx{Value: time.Now().UnixNano()}
}

func (s *MyTestSuite) TestOne(t *gotest.T, ctx *MethodParallelCtx) {
    gotest.NotZero(t, ctx.Value)
}

func (s *MyTestSuite) TestTwo(t *gotest.T, ctx *MethodParallelCtx) {
    gotest.NotZero(t, ctx.Value)
}
```

The returning `BeforeEach` pattern ensures each parallel method operates on its own isolated state.

### Type-Safe Assertions

Functional API with compile-time type safety:

```go
gotest.Equal(t, expected, actual)            // [T any] — cross-type comparison is a compile error
gotest.NoError(t, err)
gotest.ErrorIs(t, err, target)
gotest.ErrorAs[*MyError](t, err)             // returns the matched error
gotest.ErrorContains(t, err, "not found")
gotest.Contains(t, haystack, needle)
gotest.Greater(t, a, b)                      // [T cmp.Ordered]
gotest.Len(t, collection, 3)
gotest.True(t, condition)
gotest.Panics(t, func() { ... })
gotest.Regexp(t, `^start`, str)
gotest.InDelta(t, 3.14, pi, 0.01)
gotest.JSONEq(t, expected, actual)           // string, []byte, io.Reader, or any marshalable value
gotest.Eventually(t, func() bool { ... }, 5*time.Second, 100*time.Millisecond)
```

Unwrap `(T, error)` or `(T, bool)` pairs in test setup:

```go
conn := gotest.Must(db.Connect(ctx))
val  := gotest.Must(cache.Get(key))
```

All assertions work with both `*gotest.T` (suites) and `*testing.T` (standalone tests).

### Data-Driven Tests

Iterator API with compile-time type safety (recommended):

```go
func (s *Suite) TestParsing(t *gotest.T) {
    for it, tc := range gotest.Each(t, []struct {
        Desc  string
        Input string
        Want  int
    }{
        {Desc: "single digit", Input: "5", Want: 5},
        {Desc: "negative",     Input: "-3", Want: -3},
    }) {
        gotest.Equal(it, tc.Want, parse(tc.Input))
    }
}
```

Callback API also available via `t.Each(entries, fn)`:

```go
t.Each(cases, func(it *gotest.T, tc Case) {
    gotest.Equal(it, tc.Want, parse(tc.Input))
})
```

Each entry becomes a subtest.
Uses `Desc` or `Name` field for the test name, falls back to `#0`, `#1`, etc.

### Async Assertions

```go
// Poll until condition is met (or timeout)
t.Eventually(5*time.Second, 100*time.Millisecond, func(poll *gotest.T) {
    gotest.Equal(poll, "ready", getStatus())
})

// Assert condition holds for the full duration
t.Consistently(500*time.Millisecond, 50*time.Millisecond, func(poll *gotest.T) {
    gotest.True(poll, cache.IsValid())
})
```

Poll callbacks receive a `*gotest.T` — use the full assertion library inside.
Failures during polling are collected, not propagated, until the timeout.

### Snapshot Testing

```go
func (s *Suite) TestRender(t *gotest.T) {
    t.MatchSnapshot(render(input))           // auto-named from test
    t.MatchSnapshot(render(other), "variant") // custom snapshot name
}
```

Snapshots are stored in `testdata/__snapshots__/`.
On first run, the snapshot is created.
On subsequent runs, the output is compared.
Update all snapshots with:

```bash
gotest --update-snapshots ./...
```

### Scaffold

Generate a test suite skeleton from any Go type:

```bash
gotest scaffold ./pkg/user.UserService
# Generated: pkg/user/user_service_suite_test.go
```

### Migrate from testify/suite

```bash
gotest migrate ./...
# Migrated 12 suites across 8 packages:
#   pkg/user/user_test.go: UserSuite → UserTestSuite
```

Renames lifecycle methods, rewrites assertions, removes testify imports.

## How It Works

```
you write:          gotest generates:         go test runs:
                    (hidden, auto-cleaned)

MySuite struct      ƒƒ_psuite_test.go        func TestMySuite(t *testing.T)
  BeforeAll()   →     BeforeAll wrapper    →    t.Cleanup(AfterAll)
  TestFoo()           TestFoo wrapper            BeforeAll()
  AfterAll()          t.Run("TestFoo",...)       t.Run("TestFoo", ...)
                                                 ...
```

The generated code is what a careful developer would write by hand: `t.Run`, `t.Cleanup`, `defer`, `sync.WaitGroup`.
No reflection, no interface dispatch.

## Naming Conventions

| Convention | Meaning |
|---|---|
| `*TestSuite` suffix | Test suite struct |
| `BeforeAll` / `AfterAll` | Suite-level lifecycle |
| `BeforeEach` / `AfterEach` | Test-level lifecycle |
| `Test*` method | Test case |
| `F_` prefix | Focus (run only this) |
| `X_` prefix | Exclude (skip this) |
| `SuiteGuard()` method | Runtime-conditional suite skipping |
| `*Fixture` suffix | Package-scoped fixture |
| `*SharedFixture` suffix | Cross-package shared fixture |
| `FixtureConfig()` method | Fixture timeout/retry config |
| `SharedFixtureConfig()` method | Shared fixture timeout/retry config |
| `SuiteConfig()` method | Suite timeout/failfast config |
| `Hydrate` / `Dehydrate` | SharedFixture test-process resource reconstruction |

## Behavior Specification

View test suites as a readable behavioral specification:

```bash
gotest spec ./pkg/user -v
```

```
UserService
  Create
    when email is valid
      ✓ creates the user (8ms)
      ✓ sends a welcome email (120ms)
    when email already exists
      ✓ returns ErrDuplicate (<1ms)
  Delete
    ✓ soft-deletes the user (5ms)
    ~ hard-deletes after 30 days — SKIPPED

2 suites, 5 behaviors: 4 passed, 0 failed, 1 skipped
```

Generate a markdown specification document:

```bash
gotest spec ./... --format=md --output=docs/behavior-spec.md
```

Append spec view after normal test output:

```bash
gotest ./... -v --spec
```

## Watch Mode

Re-run tests on file changes with 200ms debounce:

```bash
gotest watch ./... -v
gotest watch ./... --spec     # watch + spec view
```

Only the affected package is re-run.
Combine with `F_` prefix for a tight feedback loop — only focused tests run on each save.

## Commands

```bash
gotest ./... -v -race          # generate, test, cleanup (default)
gotest spec ./...              # behavioral specification view
gotest watch ./... -v          # watch mode with auto-rerun
gotest scaffold ./pkg/user.Svc # generate suite skeleton from type
gotest lint ./...              # static analysis for test suites
gotest refactor toggle-focus . # toggle F_/X_ prefixes programmatically
gotest migrate ./...           # convert testify/suite to go-test
gotest generate ./...          # run code generation only (no tests)
gotest clean ./...             # remove orphaned generated files
gotest version                 # print version
gotest help                    # show help
```

All `go test` flags work unchanged: `-race`, `-cover`, `-count`, `-run`, `-json`, `-short`, `-timeout`, `-v`.

### Linter

Catch common mistakes in test suites with static analysis:

```bash
gotest lint ./...
```

Detects: lifecycle hook typos, value receivers on suite methods, missing `AfterAll` when `BeforeAll` exists, committed `F_` prefixes, and orphaned generated files.
Also available as a standalone binary (`gotest-lint`) compatible with `golangci-lint` via `go/analysis`.

## VS Code Extension

The **Go Test Suites** extension brings first-class IDE support: suite-aware Test Explorer, CodeLens run/debug buttons, coverage gutters, watch mode, spec view, focus/exclude quick fixes, and suite scaffolding.
Available on the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) and [Open VSX](https://open-vsx.org/extension/mvrahden/gotest).
Install via `code --install-extension mvrahden.gotest`.

## License

MIT
