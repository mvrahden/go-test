# gotest

<p align="center">
  <img src="https://raw.githubusercontent.com/mvrahden/go-test/main/site/static/gopher.png" alt="gotest gopher" width="360" />
</p>

[![Tests](https://github.com/mvrahden/go-test/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/mvrahden/go-test/actions/workflows/test.yml?query=branch%3Amain)
[![Quality](https://github.com/mvrahden/go-test/actions/workflows/quality.yml/badge.svg?branch=main)](https://github.com/mvrahden/go-test/actions/workflows/quality.yml?query=branch%3Amain)
[![Coverage](https://raw.githubusercontent.com/mvrahden/go-test/ci/badges/coverage.svg)](https://github.com/mvrahden/go-test/actions/workflows/test.yml?query=branch%3Amain)
[![Go Reference](https://pkg.go.dev/badge/github.com/mvrahden/go-test.svg)](https://pkg.go.dev/github.com/mvrahden/go-test)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Specification-driven test suites for Go with isolation and parallelism as first-class citizens.

Write test suites as Go structs.
`gotest` generates the lifecycle wiring, `t.Run` nesting, and process isolation that you'd write by hand.
What runs is standard `go test`.
What you read back is a behavioral specification.

No third-party runtime dependencies. No reflection in discovery or registration. Pure code generation.

## Install

Add gotest to your module as a Go tool. Its version and its Go toolchain then follow your `go.mod`:

```bash
go get -tool github.com/mvrahden/go-test/cmd/gotest@latest
go tool gotest ./...
```

`go get -tool` pins the CLI and the `pkg/gotest` runtime to the same release, and `go tool` rebuilds the CLI with your module's Go. Upgrading is the same command again. Avoid a global `go install` binary: it drifts from `go.mod` on both axes, and gotest refuses to run on that drift.

The examples below write `gotest` for brevity; with the tool directive, read `go tool gotest`. The VS Code extension and the GitHub action run the same CLI your `go.mod` selects.

## 30-Second Example

Write a test suite:

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
            err := s.svc.Create("alice@example.com")
            gotest.NoError(it, err)
            err = s.svc.Create("alice@example.com")
            gotest.ErrorIs(it, err, ErrDuplicate)
        })
    })
}
```

Run:

```bash
gotest ./... -v
```

Standard `go test` output:

```
=== RUN   TestUserServiceTestSuite
=== RUN   TestUserServiceTestSuite/TestCreate
=== RUN   TestUserServiceTestSuite/TestCreate/creates_a_user_with_valid_input
=== RUN   TestUserServiceTestSuite/TestCreate/when_email_already_exists
=== RUN   TestUserServiceTestSuite/TestCreate/when_email_already_exists/returns_ErrDuplicate
--- PASS: TestUserServiceTestSuite (0.01s)
```

The same suites render as a behavioral specification:

```bash
gotest spec ./pkg/user
```

```
UserService (12ms)
  Create (12ms)
    ✓ creates a user with valid input (11ms)
    when email already exists (<1ms)
      ✓ returns ErrDuplicate (<1ms)

1 suites, 2 behaviors: 2 passed
```

No generated code leaks into your workflow.
`gotest` creates it before tests run - invisible to the project's filetree.

## Why gotest?

Go's `testing` package gives you `func TestX(t *testing.T)` and nothing more.
Setup/teardown logic is copy-pasted or buried in `TestMain`.
As test suites grow, organization becomes a discipline problem rather than a tooling one.

**testify/suite** solves organization but adds runtime reflection, interface dispatch, and a `suite.Run(t, new(MySuite))` ceremony in every file.
Test output is standard — but the mechanism behind it isn't.

**gotest** takes a different approach:

- **Specification-driven.** BDD vocabulary (`When`/`It`) structures tests as readable behavioral contracts. `gotest spec` renders them as documentation — in the terminal, as markdown, or as structured JSON. Always in sync, never stale.
- **Isolated by default.** Each suite runs in its own process. Each test gets fresh state through lifecycle hooks. Shared mutable state between tests isn't a discipline problem — it's structurally impossible.
- **Safely parallel.** Suite-level parallelism is automatic. Method-level is opt-in. Because isolation is built in, parallel tests can't interfere with each other.

Under the hood, `gotest` generates the same `t.Run`, `t.Cleanup`, and `defer` code you'd write by hand.
Generated files never touch your source tree — they're created hidden before tests run and cleaned up after.

## How It Works

```
you write:          gotest generates:         go test runs:
                    (hidden, auto-cleaned)

MySuite struct      gotest_psuite_test.go     func TestMySuite(t *testing.T)
  BeforeAll()   →     lifecycle wiring    →     t.Cleanup(AfterAll)
  TestFoo()           t.Run nesting              BeforeAll()
  AfterAll()          process isolation          t.Run("TestFoo", ...)
```

The generated code is what a careful developer would write by hand: `t.Run`, `t.Cleanup`, `defer`, `t.Parallel`.
No reflection or interface dispatch in the generated wiring.

## Specification

Tests are structured as behavioral specifications using BDD vocabulary.
`gotest spec` renders the structure as readable documentation — in the terminal, as markdown, or as structured JSON for CI reports, AI conversations, and documentation pipelines.

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
Write the condition, not the connective: the spec renders `When("input is valid")` as *when input is valid*, and the ✓ glyph plays the role of "it".

### Behavior Specification

View test suites as a readable behavioral specification:

```bash
gotest spec ./pkg/user -v
```

```
UserService (133ms)
  Create (128ms)
    when email is valid (128ms)
      ✓ creates the user (8ms)
      ✓ sends a welcome email (120ms)
    when email already exists (<1ms)
      ✓ returns ErrDuplicate (<1ms)
  Delete (5ms)
    ✓ soft-deletes the user (5ms)
    ~ hard-deletes after 30 days — SKIPPED (<1ms)

1 suites, 5 behaviors: 4 passed, 1 skipped
```

Every row shows the wall clock it occupied, never the sum of the rows beneath it — so a row that exceeds its children is time it held itself, and one that falls short of their total is children that overlapped.

Generate a markdown specification document:

```bash
gotest spec ./... --format=md --output=docs/behavior-spec.md
```

Render the spec view instead of the default output (also works in watch mode):

```bash
gotest ./... -v --spec
```

Read the spec without running it. `--static` builds the tree from the source, so nothing carries a verdict or a duration. Where a method's behaviors depend on runtime values — a `When` behind a condition, an `Each` over a table that is not a literal — the node says so and the detail goes to stderr, because a partial spec that presents itself as whole is worse than none:

```bash
gotest spec ./... --static
gotest spec ./... --static --format=md --output=docs/behavior-spec.md
```

## Isolation

Each suite runs in its own process with zero shared state.
Each test method gets fresh state through lifecycle hooks.
Resources live as suite fields, managed through `BeforeEach`/`AfterEach` — never scattered across test methods with `defer` or `t.Cleanup`.

### Lifecycle Hooks

Suite hooks accept either `*gotest.T` or `*testing.T` — you choose per method:

```go
func (s *MySuite) BeforeAll(t *gotest.T)  {} // once before all tests
func (s *MySuite) AfterAll(t *gotest.T)   {} // once after all tests
func (s *MySuite) BeforeEach(t *gotest.T) {} // before each test method
func (s *MySuite) AfterEach(t *gotest.T)  {} // after each test method
```

`*gotest.T` exposes `t.Context()` (mirrors Go 1.24's `testing.T.Context()`), plus the BDD vocabulary (`t.It()`, `t.When()`).
Use `*testing.T` for plain stdlib tests — the functional assertions (`gotest.Equal(t, ...)`) still work with either type.

You can mix freely within a single suite:

```go
func (s *MySuite) BeforeEach(t *testing.T) {} // stdlib is fine here
func (s *MySuite) TestPlain(t *testing.T)  {} // no gotest import needed
func (s *MySuite) TestRich(t *gotest.T)    {} // full DSL available
```

All hooks are optional.
`AfterAll` runs via `t.Cleanup` (LIFO).
`AfterEach` is deferred, so it runs even on `t.Fatal()`.

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

Fixture hooks receive `context.Context` and return `error` — the generated wrapper reports failures with automatic attribution (e.g., `E2ESetupFixture.BeforeAll failed: start postgres: connection refused`).

`BeforeAll`/`AfterAll` receive `context.Background()` bounded by the fixture's configured timeout.
`BeforeEach` receives the test's `t.Context()`; `AfterEach` receives `context.Background()` — cleanup must proceed even after the test context is cancelled.
Requires Go 1.25+.

Test suites reference fixtures via named pointer fields — one or more:

```go
type BatchTestSuite struct {
    Setup *E2ESetupFixture
    Cache *CacheFixture
}

func (s *BatchTestSuite) TestDispatch(t *gotest.T) {
    // s.Setup.Pool and s.Cache.Client are populated by their respective BeforeAll hooks
}
```

Fixtures support the same lifecycle hook names as suites — with `(ctx context.Context) error` signatures instead of `*gotest.T`.
`BeforeAll`/`AfterAll` run once around all suites bound to the fixture.
`BeforeEach`/`AfterEach` wrap every individual test case, running outside the suite's own hooks:

```
Fixture.BeforeEach
  Suite.BeforeEach
    TestCase
  Suite.AfterEach
Fixture.AfterEach
```

`BeforeAll` is required; the other three hooks are optional.

Fixtures compose naturally — each fixture can depend on multiple others via pointer fields.
Dependencies are set up before dependents, independent fixtures set up in parallel, and everything tears down in reverse.
If two fixtures share a common dependency, it's created once:

```go
type InfraFixture struct { Pool *pgxpool.Pool }
type CacheFixture struct { Client *redis.Client }

type APIFixture struct {
    Infra *InfraFixture   // depends on infra
    Cache *CacheFixture   // and cache — both set up before APIFixture
}
```

```
InfraFixture.BeforeEach  ─┐
CacheFixture.BeforeEach  ─┤
                          └─ APIFixture.BeforeEach
                               └─ Suite.BeforeEach → TestCase → Suite.AfterEach
                          ┌─ APIFixture.AfterEach
InfraFixture.AfterEach  ──┤
CacheFixture.AfterEach  ──┘
```

**Failure reporting.**
Fixture hook failures are reported with automatic attribution.
`BeforeEach`/`AfterEach` failures appear in test output — attributed to the fixture and the failing hook:

```
--- FAIL: TestBatchTestSuite/TestDispatch
    E2ESetupFixture.BeforeEach failed: connection refused
```

`BeforeAll`/`AfterAll` failures are fatal — they fail all fixture-bound tests with automatic attribution:

```
FAIL: E2ESetupFixture.BeforeAll failed after 2 attempt(s): start postgres: connection refused
```

For cross-package shared state (e.g. a database container shared across integration test packages), use `*SharedFixture` suffix.
SharedFixtures can depend on other SharedFixtures via pointer fields — `BeforeAll` runs in dependency order, and suites start as soon as their specific dependencies are ready:

```go
type SchemaSharedFixture struct {
    Postgres *PostgresSharedFixture   // dependency — Postgres starts first
    Version  string
}
```

See [docs/design/fixtures.md](docs/design/fixtures.md) for the full reference.

## Parallelism

Isolation makes parallelism safe.
Parallelism makes tests fast.
`gotest` gives you both.

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

### Concurrency control

These two dimensions — suite processes and method goroutines — multiply.
Left unchecked, that product grows quadratically with CPU count and saturates the machine.
`gotest` manages this automatically with a linear concurrency budget.

By default the budget is `2 × GOMAXPROCS`.
The runner caps suite processes at `GOMAXPROCS` (one per core) and distributes the remaining budget as `-test.parallel` to each subprocess.
On a 4-core machine with 10 suites this means 4 concurrent processes, each running 2 test methods at a time — 8 total, not 32.

Override with `--parallel`:

```bash
gotest ./... --parallel 16          # total budget of 16
gotest ./... --parallel 12 -parallel 4  # 4 per suite, 3 processes (12/4)
gotest ./... -parallel 8            # explicit per-suite, no budget management
```

| Flags | Suite processes | Methods per suite | Total |
|:------|:---------------:|:-----------------:|:-----:|
| *(none)* | `min(S, GOMAXPROCS)` | `budget / inter` | `2 × GOMAXPROCS` |
| `--parallel N` | `min(S, GOMAXPROCS, N)` | `N / inter` | `N` |
| `-parallel M` | `2 × GOMAXPROCS` | `M` | `2 × GOMAXPROCS × M` *(compat)* |
| `--parallel N -parallel M` | `min(S, GOMAXPROCS, N/M)` | `M` | `~N` |

Where *S* is the number of suites and *inter* is the computed process count.
Under `-race`/`-msan`/`-asan` the default budget halves (as does compile concurrency) — instrumentation at least doubles the CPU cost per instruction stream, and keeping the uninstrumented defaults would oversubscribe the machine. An explicit `--parallel`/`--compile-parallel` always wins over this heuristic.

## Testing Toolkit

### Type-Safe Assertions

Functional API with compile-time type safety:

```go
// Equality
gotest.Equal(t, expected, actual)            // [V any] — deep equality; cross-type = compile error
gotest.NotEqual(t, expected, actual)         // [V any] — deep inequality

// Boolean
gotest.True(t, condition)
gotest.False(t, condition)

// Zero / nil
gotest.Zero(t, value)                        // [V comparable] — value == zero value for type
gotest.NotZero(t, value)                     // [V comparable] — also covers pointer/interface nil
gotest.Nil(t, object)                        // non-comparable nilables (slices, maps, funcs); type-guarded
gotest.NotNil(t, object)

// Errors
gotest.NoError(t, err)
gotest.Error(t, err)                         // err != nil
gotest.ErrorIs(t, err, target)
gotest.ErrorAs[*MyError](t, err)             // returns the matched error
gotest.ErrorContains(t, err, "not found")

// Collections
gotest.Empty(t, object)                      // nil, or len == 0
gotest.NotEmpty(t, object)
gotest.Len(t, collection, 3)
gotest.Contains(t, haystack, needle)         // substring, element, or map key
gotest.NotContains(t, haystack, needle)
gotest.ElementsMatch(t, a, b)               // [V comparable] — same elements, any order
gotest.Subset(t, list, subset)              // [V comparable] — all subset elements in list

// Ordering
gotest.Greater(t, a, b)                      // [V cmp.Ordered]
gotest.GreaterOrEqual(t, a, b)               // [V cmp.Ordered]
gotest.Less(t, a, b)                         // [V cmp.Ordered]
gotest.LessOrEqual(t, a, b)                  // [V cmp.Ordered]

// Numeric
gotest.InDelta(t, 3.14, pi, 0.01)

// Strings & patterns
gotest.Regexp(t, `^start`, str)

// JSON
gotest.JSONEq(t, expected, actual)           // string, []byte, io.Reader, or any marshalable value

// Time
gotest.TimeWithin(t, expected, actual, tol)  // times within tolerance
gotest.TimeIsNow(t, ts, tolerance)           // timestamp ≈ now

// Panics
gotest.Panics(t, func() { ... })             // returns recovered value

// Failure
gotest.Fail(t, "unreachable")                // immediate unconditional failure

// Async polling
gotest.Eventually(t, 5*time.Second, 100*time.Millisecond, func(poll *gotest.R) { ... })
gotest.Consistently(t, 500*time.Millisecond, 50*time.Millisecond, func(poll *gotest.R) { ... })

// Panic-safe goroutines
wait := gotest.Go(t, func() { ... })         // captures a goroutine panic and re-raises it on the test's goroutine
```

Unwrap `(T, error)` or `(T, bool)` pairs in test setup:

```go
conn := gotest.Must(db.Connect(ctx))
val  := gotest.Must(cache.Get(key))
```

All assertions work with `*gotest.T` (suites), `*testing.T` (standalone tests), and `*gotest.R` (polling callbacks).
Assertion failures automatically trace back to the test call site — no `t.Helper()` calls needed in helper functions.

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

Each entry becomes a subtest.
Uses `Desc` or `Name` field for the test name, falls back to `#0`, `#1`, etc.

### Async Assertions

```go
// Poll until condition is met (or timeout)
gotest.Eventually(t, 5*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
    gotest.Equal(poll, "ready", getStatus())
})

// Assert condition holds for the full duration
gotest.Consistently(t, 500*time.Millisecond, 50*time.Millisecond, func(poll *gotest.R) {
    gotest.True(poll, cache.IsValid())
})
```

Poll callbacks receive a `*gotest.R` — an assertion recorder that captures failures without propagating them to the test runner.
The full assertion library works with `*R` just as it does with `*T` or `*testing.T`.
Failures are collected until the timeout; only the final outcome is reported.

### Snapshot Testing

```go
func (s *Suite) TestRender(t *gotest.T) {
    gotest.MatchSnapshot(t, render(input))            // auto-named from test
    gotest.MatchSnapshot(t, render(other), "variant") // custom snapshot name
}
```

Snapshots are stored in `testdata/__snapshots__/`.
On first run, the snapshot is created.
On subsequent runs, the output is compared.
Snapshot entries are written in deterministic order and are thread-safe — parallel test methods can use `MatchSnapshot` without coordination.
Update all snapshots with:

```bash
gotest --update-snapshots ./...
```

Or when running `go test` directly:

```bash
GOTEST_UPDATE_SNAPSHOTS=1 go test ./...
```

## Benchmarking

Benchmark methods live on the same suites as tests — same struct, same lifecycle, own subcommand.

### Authoring

```go
type ParserTestSuite struct {
    p *Parser
}

func (s *ParserTestSuite) BeforeEach(t *gotest.T) { s.p = NewParser() }

func (s *ParserTestSuite) BenchmarkParse(b *gotest.B) {
    doc := loadTestdata("large.json")
    for b.Loop() {
        s.p.Parse(doc)
    }
}
```

`Benchmark*` methods take `*gotest.B` (or `*testing.B`) and honor the same `F_`/`X_` focus/exclude prefixes as test methods.
`BeforeEach`/`AfterEach` run once per benchmark method, outside the timing window — the generated wrapper stops the timer before `BeforeEach`, starts and resets it right before your method runs, and stops it again before `AfterEach`.
Use `b.Loop()` (Go 1.24+) rather than `b.N` — it excludes setup before the loop from timing automatically.

The suite must be named `*TestSuite`, even if it only has benchmarks — a struct without that suffix is never discovered, so its `Benchmark*` methods are silently dropped.
Benchmarks can't coexist with a returning `BeforeEach` (its context type can't thread through `*gotest.B`), and every lifecycle hook on a benchmark suite must take `*gotest.T`, not `*testing.T`.
Fixture-bound suites work — fixtures hydrate before benchmarks run — but a fixture with `BeforeEach`/`AfterEach` bound to a suite that has `Benchmark*` methods is rejected at generation time: per-method fixture hooks aren't supported for benchmarks.

### Running

```bash
gotest bench ./...                     # all benchmarks
gotest bench ./pkg/parser -run Parse   # filter by suite name
gotest bench ./pkg/parser -bench Parse # filter by benchmark name
```

`gotest ./...` never runs benchmarks — use `gotest bench` explicitly.

Benchmark suites always run serially, one suite process at a time, regardless of `--parallel`/`-test.parallel`.
Running benchmarks concurrently makes their timing numbers meaningless — the runner disables streaming and compiles everything up front so `go test -c` never competes with a running benchmark for CPU.
A fresh process per suite is also a methodological win, not just an implementation detail: GC pressure from one benchmark can't pollute another's numbers.

`-test.benchmem` is on by default — every benchmark line reports `B/op`/`allocs/op` alongside `ns/op`.
`-benchtime` and `-count` are forwarded to `go test` unchanged.
`-coverprofile` works; `--min` (coverage gate) isn't available in bench mode.
`-run` and `-bench` both filter which suites run, both matched against each suite's `Benchmark<SuiteName>` wrapper function — `-run` the same way it scopes suites for `gotest ./...`, `-bench` by benchmark function name.
Given both, a suite must match both to run (AND semantics).
`-run Test<Suite>`-style values match nothing in bench mode — filter by the suite name itself, not the `Test` prefix.

With no matching benchmarks, `gotest bench` prints `no benchmarks found` and exits 0.

### Spec view

```bash
gotest bench --spec ./examples/notification -benchtime=10x
```

```
BenchmarkNotificationDispatchBench
  ✓ Dispatch  810.6 ns/op · 596 B/op · 2 allocs/op

1 suites, 1 benchmarks: 
```

Each line reports `ns/op`, `B/op`, and `allocs/op` — the same numbers `go test -bench` prints, rendered as a spec.

## Fuzzing

Fuzz methods live on the same suites as tests — same struct, same lifecycle, own subcommand.

### Authoring

```go
type ParserTestSuite struct {
    p *Parser
}

func (s *ParserTestSuite) BeforeEach(t *gotest.T) { s.p = NewParser() }

func (s *ParserTestSuite) FuzzParse(f *gotest.F) {
    f.Add(`{"a":1}`)
    f.Fuzz(func(t *gotest.T, input string) {
        doc, err := s.p.Parse(input)
        if err != nil {
            return // rejecting invalid input is fine
        }
        out, err := doc.Marshal()
        gotest.NoError(t, err)
        gotest.JSONEq(t, input, out)
    })
}
```

A fuzz method takes `*gotest.F` and calls `f.Fuzz` with a callback whose first parameter is `*gotest.T`; any number of fuzzed arguments follow, exactly as with `(*testing.F).Fuzz`.
`f.Add(...)` registers seeds, and the `F_`/`X_` prefixes focus and exclude fuzz methods like tests.
The suite's `BeforeEach` and `AfterEach` run around every single execution, seeds included, not once per target, so each execution starts from fresh state.

Because `f.Fuzz` takes `any`, the callback's shape is checked when gotest generates, and the message names the method:

- the callback must take a leading `*gotest.T` and return nothing;
- `f.Fuzz` may be called once per method;
- `*testing.F` is rejected, since the per-execution lifecycle needs `gotest.F`.

The suite-shape rules match benchmarks, with fuzz-specific messages: a returning `BeforeEach` is rejected (move fuzz targets to a dedicated suite), every lifecycle hook must take `*gotest.T`, and a fixture with `BeforeEach`/`AfterEach` cannot bind to a fuzzing suite (per-execution fixture hooks are not supported).

Each `Fuzz*` method becomes one top-level function named `Fuzz<SuiteIdentifier>_<MethodName>`, e.g. `FuzzParserTestSuite_FuzzParse`, because Go's fuzzing engine targets one `FuzzX` symbol per run.

### Struct arguments

Go's fuzzing engine accepts exactly fifteen argument types — and the hand-written wire and message types you most want to fuzz are structs.
`f.Fuzz` accepts a struct — or any named type over a native one, like `type Priority int` — and codegen fans it out: one engine argument per leaf field, reassembled into your type before every execution.
The engine still sees only types it understands, but it sees the *fields*, so its mutator moves each one independently instead of chewing on one opaque blob.
The supported set is deliberately strict: structs of exported fields with simple shapes.
Maps, unexported fields (which excludes `time.Time` and protobuf-generated types), interfaces, and recursion are refused at generation time with a suggested alternative — the full table is below.

```go
func (s *UserServiceTestSuite) FuzzCreate(f *gotest.F) {
    f.Add(CreateUserRequest{Email: "a@b.c", Age: 30})
    f.Fuzz(func(t *gotest.T, req CreateUserRequest) {
        gotest.NoError(t, req.Validate())
    })
}
```

What fans out:

- `string`, `bool` and `[]byte` fields pass through unchanged.
- Numbers ride as fixed-width bytes, which gives the mutator its richest operators: bit flips, interesting values, boundary values. Why that beats a native number is in ARCHITECTURE.md, "Code Generation".
- Nested structs, pointers and small arrays flatten into the same tuple; a variable-length slice of non-bytes rides as one packed `[]byte`.
- Every position of a multi-argument callback fans on its own: `f.Fuzz(func(t *gotest.T, h Header, n int))` fans `Header` into its leaves and `n` into one leaf. Two values as arguments and two values in a struct reach the engine identically; use a struct when the values form a concept.

Seeds stay plain Go literals.
`f.Add` buffers them and `f.Fuzz` explodes each one through the target's own fan; a seed of the wrong type is rejected with the position named (`seed #1: value 1: f.Add was given []byte, but this fuzz target takes CreateUserRequest`).

Crashers stay readable.
A failing execution prints the decoded value beside the failure, `CreateUserRequest{Email: "a@\x00", Age: -1}` rather than corpus bytes, and `gotest fuzz promote` splices that literal back into the method as a typed `f.Add(...)` seed.
Every shape the fan accepts renders as a literal, pointers included (`&[]int{5}[0]`, since `&5` is not Go and `new(5)` needs Go 1.26).

**A struct target's corpus files are bound to its field order.**
Fields map to leaves in declaration order, so an entry on disk means whatever the current field list says:

- Adding or removing a field changes the leaf count. Go's engine rejects every old entry, and gotest names the drift first: a pre-flight check on plain runs and on `gotest fuzz` prints which entry no longer fits, what it holds and what the target now takes.
- Swapping two fields of the same kind is silent: the entry loads and quietly becomes a different test.

The rule that follows: promote crashers to `f.Add` seeds instead of committing corpus files for struct targets.
A promoted seed is source and survives any change to the encoding.
The `fuzz-struct-corpus` lint rule flags on-disk entries for a shape-bound target and points at `gotest fuzz promote`.

Refused at generation time, with the alternative named in the message, because generated code that cannot round-trip faithfully would lie:

| Refused | Do this instead |
|---|---|
| structs with unexported fields | fuzz the constructor's input, or declare a local wrapper struct |
| `map` fields | fuzz a slice of key/value pairs and build the map in the callback |
| interfaces, channels, funcs | — |
| recursive types | — |
| `time.Time` and friends (unexported internals) | fuzz an `int64` and convert in the callback |

Each rejection names the field path and its type, e.g. `fuzz target FuzzUserServiceTestSuite_FuzzCreate: CreateUserRequest.mu (sync.Mutex) is not fuzzable — unexported fields cannot be set — fuzz the constructor's input, or declare a local wrapper struct`.
Generation is the only place this can be caught: `go vet` checks direct `(*testing.F).Fuzz` calls only, and `f.Fuzz` takes `any`.

### Seed harvesting

At generation time, gotest mines each `Fuzz*` method's package for literal primitive values that already flow into the function under fuzz — table-test rows (`gotest.Each`) and direct call-site literals in `_test.go` files — and injects them as additional `f.Add(...)` seeds in the generated wrapper, on top of whatever you added by hand.
Your existing valid-input examples become a starting corpus for free — coverage-guided mutation from real inputs finds interesting states far faster than starting from `""`.

Harvesting only scans test files, never production code, and only lifts literal shapes — basic literals, `true`/`false`, a unary-minus of one, or a single-arg conversion of one — never computed expressions.
It's on by default; disable it for one run with `--no-harvest`, or persistently with `fuzz: harvest: false` in `.gotest.yml` (see [Project Configuration](#project-configuration)).

### Seed replay

```bash
gotest ./...
```

```
=== RUN   FuzzParserTestSuite_FuzzParse
=== RUN   FuzzParserTestSuite_FuzzParse/seed#0
--- PASS: FuzzParserTestSuite_FuzzParse (0.00s)
    --- PASS: FuzzParserTestSuite_FuzzParse/seed#0 (0.00s)
```

Every seed added via `f.Add`, plus any crasher corpus already discovered under `testdata/fuzz/`, replays as an ordinary subtest under a plain run — `gotest ./...`, `gotest spec`, `gotest watch`, and `gotest summary` all do this, at zero extra cost, with no `-fuzz` flag involved.
For a **pass-through** target, committing a crasher's corpus file makes it a permanent regression test; for a **shape-bound** target, run `gotest fuzz promote` instead — its corpus files are bound to the type's field order (the `fuzz-struct-corpus` lint rule will remind you), while a promoted `f.Add` literal is ordinary Go source.
A user-supplied `-run` filter wins over this widening — fuzz seeds only replay if a generated `Fuzz<Suite>_<Method>` name happens to match the filter on its own merit.

In the spec view the target sits under its suite as a property beside the examples — `✓ Parse  2 seeds` — and opens up to name the entry when a seed or corpus file fails; the trailer counts it as a fuzz target, never as a stdlib test.
`gotest spec --static` lists fuzz targets the same way, read from source, so the declared specification states the properties too; on the wire the node's `kind` is `fuzz`.

One caveat to state plainly: suite fuzz methods exist only through gotest's generated wrappers.
Plain `go test` (and external fuzzing infrastructure like OSS-Fuzz, which needs on-disk `FuzzXxx` functions) never sees them, and promoted seeds replay only under gotest runs.
If a target must also run under stock tooling, write it as a top-level `func FuzzXxx(*testing.F)` — `gotest.NewF(f, nil, nil, nil)` and `f.Fuzz` work there too, as plain library calls bound by reflection, though with native argument types only: fans exist solely in generated suite wrappers.

### Running

```bash
gotest fuzz ./...                     # one minute, shared across all targets
gotest fuzz --for=5m ./...            # about five minutes across all targets
gotest fuzz --for=1m --jobs=2 ./...   # two targets at a time
gotest fuzz --target=FuzzParserTestSuite_FuzzParse ./pkg/parser
```

`gotest fuzz` finds every generated `Fuzz<Suite>_<Method>` target and runs each as its own `go test -fuzz=...` process.
This is the one gotest subcommand that does not reuse the compiled suite binary: `cmd/go` weaves fuzz instrumentation in only when `-fuzz` is present on that `go test` invocation, so a binary from `go test -c` would fuzz without coverage guidance.
Each target pays its own compile and gets a real search in return.

The budget:

- `--for` is the session's wall clock, one minute by default. Each target's `-fuzztime` share is `--for × min(--jobs, targets) / targets`, so concurrent waves add back up to about `--for`. Shares never drop below 10s; a small `--for` on many targets stretches the session instead, and gotest says so. The resolved schedule and the deadline it implies print before the search starts.
- `--for` is the only clock. The deadline follows it with headroom for builds, a `.gotest.yml` `timeout` never caps a search, and `gotest fuzz` refuses `--timeout` rather than let two clocks compete.
- `--for=0` removes the budget: targets fuzz until interrupted. Only the first `--jobs` targets ever run then, since each holds its slot, and gotest says so up front.
- `--jobs` caps concurrent targets (default `max(1, GOMAXPROCS/2)`). A target whose slot never opens before the deadline is reported as `[<Func>] skipped: session ended before this target started`, and the closing summary names the targets that did not get their full share.
- `--target=<Fuzz...>` narrows a session to one wrapper; an unmatched name lists the available ones. This is what the VS Code extension invokes: fuzz methods get **Fuzz** / **Debug Seeds** CodeLenses, budgeted cancellable sessions, crasher notifications wired to triage/promote/debug, and Test Explorer items whose runs replay seeds (see `vscode-gotest/README.md`).

Ending by deadline or interrupt is the normal end of an open-ended search, not a failure.
The session exits 0 unless something was found: a failing target, or a new crasher file, detected by comparing each target's `testdata/fuzz/<Func>/` directory before and after, so a crash counts even when the deadline killed the process mid-report.

Output streams live, line by line, each line prefixed `[<Func>] `:

```
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 0s, gathering baseline coverage: 0/76 completed
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 0s, gathering baseline coverage: 76/76 completed, now fuzzing with 6 workers
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 3s, execs: 393973 (131314/sec), new interesting: 0 (total: 76)
[FuzzNotificationServiceTestSuite_FuzzTrim] fuzz: elapsed: 10s, execs: 1407280 (131111/sec), new interesting: 1 (total: 77)
[FuzzNotificationServiceTestSuite_FuzzTrim] PASS
[FuzzNotificationServiceTestSuite_FuzzTrim] ok  	github.com/mvrahden/go-test/examples/notification	10.115s
```

On a crashing input the session exits 1 and names each new corpus file, `[<Func>] new crasher: <dir>/testdata/fuzz/<Func>/<hash>`, followed by the two commands that handle it: `gotest fuzz triage` to see the decoded input and `gotest fuzz promote` to keep it as a typed seed that replays on every ordinary run.
A target that fails without writing a new file (a failing seed or existing corpus entry) is called out as such; it reproduces on a regular `gotest` run.

Every session closes with one line, `fuzzed 5 targets in 20.3s: 7,012,345 execs, 3 new interesting inputs, no crashers`.
Under GitHub Actions the same session lands in the step summary as a table with one row per target.

"Interesting" inputs are the ones that reached new coverage.
Go keeps them in its build cache, under `fuzz/<package>/<Func>/` in `go env GOCACHE`, never beside your code.
The next session resumes from them, which is why a second run starts with a larger baseline; `go clean -fuzzcache` starts over.
They are the search's own memory, not something to commit: the seeds you write and the crashers you promote are the corpus that travels with the code.
The engine's `To re-run: go test -run=...` advice is dropped from the stream, since plain `go test` cannot see a generated target; the hint after a crasher names the working commands.

With no `Fuzz*` methods anywhere, `gotest fuzz` prints `no fuzz targets found` and exits 0 without invoking `go test`.

### Triage and promote

```bash
gotest fuzz triage ./...     # re-run every discovered crasher, report pass/fail
gotest fuzz promote ./...    # splice crashers into f.Add(...) seeds
```

`gotest fuzz triage` scans each target's `testdata/fuzz/<Func>/` directory (a plain filesystem scan — no `go test -fuzz` invoked) and re-runs every corpus entry found there via `go test -run='^<Func>/<hash>$'`, printing its decoded input and either the panic/failure cause or `status: no longer failing`.
Exits 1 if any crasher's re-run still fails, 0 otherwise.

`gotest fuzz promote` does the same discovery, then splices each crasher's input into its originating `Fuzz*` method as a permanent `f.Add(...)` seed — an AST-level source edit via `internal/refactor`, inserted after the method's last existing `f.Add` call — and deletes the crasher file once the splice succeeds.
The crasher is now a committed regression test that replays for free on every ordinary run, same as any seed under [Seed replay](#seed-replay).
A crasher whose originating method can't be located with confidence is skipped with a warning and left in place; promote never partially edits source.

Struct-typed targets work the same way: triage prints the decoded struct on the `input:` line, and promote splices it as a typed `f.Add(T{...})` seed rather than an opaque byte string.
See [Struct arguments](#struct-arguments) for the shapes that carry a literal and the raw-bytes fallback for those that don't.

### Known limitation

Top-level stdlib fuzz functions — a bare `func FuzzXxx(f *testing.F)` not attached to any suite — are not discovered or run by the gotest runner at all.
They only run via `go test -run`/`go test -fuzz` directly; `gotest ./...`, `gotest fuzz`, and every other gotest subcommand silently ignore them.
Only `Fuzz*` methods on `*TestSuite` structs participate in seed replay and the `gotest fuzz` orchestrator.

### Worked example

[`examples/fuzzing`](examples/fuzzing) fuzzes a message broker's wire-frame codec: a struct-typed round-trip target, a `[]byte` target asserting the decoder never panics on adversarial input, a string idempotence target, and two two-argument targets — one over two strings, one mixing a struct with a string.
Its README records a real crash-to-regression-test cycle — the fuzzer finding a framing bug, `triage` decoding it, `promote` splicing it back as a typed seed.

## Configuration

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
        Timeout:      1 * time.Minute,
        SetupTimeout: 2 * time.Minute,
        FailFast:     true,
        Parallel:     false,
    }
}
```

The returned config is used as-is — a zero (or omitted) duration means no timeout, and without a `SuiteConfig()`/`FixtureConfig()` method the defaults apply.
Start from a preset to combine defaults with overrides (`cfg := gotest.DefaultSuiteConfig(); cfg.Parallel = true; return cfg`).

`Exclusive: true` schedules the suite's process strictly alone: after every non-exclusive suite has finished, one exclusive suite at a time, in deterministic order.
Use it for suites whose verdicts depend on wall-clock behavior or contended resources (timing budgets, containers, ports, heavy child builds) — a budget verdict taken on a saturated machine is not a verdict you can act on.
Like `Parallel`, it is resolved statically by the generator and affects scheduling only.

Preset constructors for common scenarios:

| Preset | Timeout | SetupTimeout | Retries | RetryDelay | Use case |
|--------|---------|--------------|---------|------------|----------|
| `DefaultFixtureConfig()` | 2 min | — | 0 | — | Standard fixtures |
| `ContainerFixtureConfig()` | 5 min | — | 1 | 5 sec | Testcontainers, image pulls |
| `DefaultSuiteConfig()` | 30 sec | 30 sec | — | — | Unit/integration tests |
| `IntegrationSuiteConfig()` | 2 min | 5 min | — | — | Heavier integration tests |

### Project Configuration

Project-level defaults live in `.gotest.yml` at the repository root (or nearest parent with a `go.mod`).
CLI flags always take precedence over config values; omitted keys fall back to defaults (an explicit `0` on a duration key disables the deadline).

```yaml
# .gotest.yml
tags: "integration,e2e"
timeout: 15m
setup-timeout: 5m
min-coverage: 80
parallel: 12
debounce: 500ms
lint:
  skip:
    - stdlib-test
    - testify
```

| Field | Type | CLI flag | Description |
|-------|------|----------|-------------|
| `tags` | string | `-tags` | Comma-separated build tags |
| `timeout` | duration | `--timeout` | Global pipeline deadline (default: 15m, 0 to disable) |
| `setup-timeout` | duration | `--setup-timeout` | Total budget for shared fixture setup (default: 2m, 0 to disable) |
| `min-coverage` | int | `--min` | Minimum coverage percentage (0–100) |
| `parallel` | int | `--parallel` | Total concurrent test method budget |
| `compile-parallel` | int | `--compile-parallel` | Concurrent compilation processes (default: NumCPU, auto-halved for -race/-msan/-asan) |
| `debounce` | duration | `--debounce` | Watch mode re-run delay |
| `lint.skip` | list | — | Non-integrity lint rules to disable project-wide |

## Test Selection

### Focus and Exclude

```go
type F_UserServiceTestSuite struct { ... }  // F_ prefix: only this suite runs
type X_BrokenTestSuite struct { ... }       // X_ prefix: this suite is skipped

func (s *MySuite) F_TestCreate(t *gotest.T) {} // focus a single test
func (s *MySuite) X_TestFlaky(t *gotest.T)  {} // exclude a single test
```

Use `--ci` in CI to fail the build if any `F_` prefix slipped through and to enable snapshot read-only mode (missing baselines fail instead of being generated):

```bash
gotest --ci ./... -v -race
```

CI mode is auto-detected: when the standard `CI` environment variable is set and `GOTEST_CI` is unset, `--ci` is enabled automatically. Set `GOTEST_CI=0` to opt out.

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

## Tooling

### Watch Mode

Re-run tests on file changes with 200ms debounce:

```bash
gotest watch ./... -v
gotest watch ./... --spec     # watch + spec view
```

Only the affected package is re-run.
Combine with `F_` prefix for a tight feedback loop — only focused tests run on each save.

### Scaffold

Generate a test suite skeleton from any Go type:

```bash
gotest scaffold ./pkg/user.UserService
# Generated: pkg/user/user_service_suite_test.go
```

### Migrate from testify/suite

```bash
gotest migrate ./...
# Migrated 12 suites:
#   pkg/user/user_test.go: UserSuite → UserTestSuite
```

Renames lifecycle methods, rewrites assertions, removes testify imports.

### Linter

Catch common mistakes in test suites with static analysis:

```bash
gotest lint ./...
```

Twenty-seven rules in three tiers:

- **Integrity** — violations can make test outcomes unreliable or leak resources: committed `F_` prefixes, value receivers on suite methods, lifecycle hook typos, `BeforeAll` without `AfterAll`, `X_` prefixes on lifecycle hooks, wrong test signatures, suite-lifecycle bypasses via `t.T()` (`Cleanup`/`Parallel`/`Run`), outer `t` inside `Eventually`/`Consistently` callbacks, `Nil`/`Empty` assertions on types their runtime guards reject, reads of shared fixtures a suite never declared (window scheduling only starts what is declared), and generated files checked into version control.
- **Expressiveness** — the test is correct but its syntax can be improved: simplifiable assertions (`True(t, a == b)` → `Equal`, `Len(t, x, 0)` → `Empty`, …), redundant assertions, `if cond { Fail(...) }` guards that an assertion expresses directly, unnecessary `t.T()` escapes, and `When("when …")`/`It("it …")` descriptions that spell the word the spec already supplies. `-fix` applies the safe rewrites.
- **Migration** — adoption aids for codebases moving to gotest: stdlib test functions and testify imports; coexistence is legitimate.

Suppress per line with `//nolint:<rule>` (same line or the comment block directly above); expressiveness and migration rules can also be disabled project-wide via `.gotest.yml` (`lint.skip`). See the [design spec](docs/design/spec.md#linter) for the full rule table.
There is no golangci-lint plugin — run `gotest lint` as its own CI step alongside your existing linter. Inside GitHub Actions (or with `--github`), findings additionally surface as inline PR annotations and a step-summary table. (The standalone `gotest-lint` binary was retired; `gotest lint` accepts the same targets and driver flags, and `pkg/lint` exports the analyzer for external `go/analysis` drivers.)

## GitHub Actions

Use the official action for CI pipelines with failure summaries, inline PR annotations, and coverage reporting:

```yaml
- uses: mvrahden/go-test@v1
  with:
    packages: ./...
    race: true
    coverage: true
    min-coverage: 80
```

By default (`version: gomod`), the action runs the `gotest` your `go.mod` selects (`go tool` when declared, else `go run -mod=mod`) — no version drift between CI and local development. Set `version: latest` or a specific tag to install a standalone binary instead.

The action emits `::error` annotations that appear inline on PR diffs and writes a markdown summary to the GitHub step summary panel.

With `fuzz: true`, a fuzz step runs after the tests (`gotest fuzz` over the same packages, for `fuzz-for` or the CLI's default minute): the session's per-target table lands in the step summary, a new crasher fails the step and its corpus file is left in the checkout, listed in the `fuzz-crashers` output for an `upload-artifact` step or a `gotest fuzz promote` follow-up. Seed replay needs no such step, since every ordinary run already replays seeds. The step also carries Go's fuzz cache between runs: it restores the latest corpus of the branch or its base and saves this run's under its own key, so a short budget compounds into a deeper search over time, main's corpus feeds every pull request, and no pull request pollutes main. `fuzz-cache: false` runs from the seeds alone. (The build cache `actions/setup-go` keeps is not enough for this: it is keyed on go.sum and never re-saved on a hit, so a corpus would only survive a dependency bump.)

With `bench: true`, a benchmark step runs after the tests (`gotest bench --spec --json`): the step summary gets the benchmark count, a per-package results table (ns/op, B/op, allocs/op), the delta table when a baseline was compared, and the gate verdict when one was set; the versioned JSON report lands in a temp file exposed as the `bench-report` output, and a breached gate fails the step with the offending keys in `bench-breached-keys`.

### Inputs

The tables below are the canonical action surface — a drift guard test keeps them in sync with `action.yml`.

| Input | Description |
|---|---|
| `packages` | Package patterns to test (default `./...`) |
| `race` | Enable the race detector (default `false`) |
| `coverage` | Enable coverage profiling and reporting (default `false`) |
| `min-coverage` | Minimum coverage percentage (0-100, fails if below) |
| `flags` | Additional gotest flags (`--double-dash` style; also forwarded to the bench step) |
| `go-test-flags` | Additional go test flags (`-single-dash` style) |
| `badge` | Render a coverage badge and, on the default branch, publish it as `coverage.svg` to `badge-branch` (needs `contents: write`; turns coverage on; default `false`) |
| `badge-branch` | Branch the coverage badge is published to; created on first use (default `ci/badges`) |
| `bench` | Run benchmarks after tests via `gotest bench --spec --json` (default `false`) |
| `bench-baseline` | Baseline JSON file to compare benchmarks against (`--against`) |
| `bench-gate` | Fail if any benchmark regresses by more than this percent (`--gate`) |
| `bench-save` | Save the run as a JSON baseline at this path (`--save`); an explicit empty string saves to `bench.baseline` from `.gotest.yml`; default `false` saves nothing |
| `fuzz` | Run a budgeted fuzz session after tests via `gotest fuzz --for`; a new crasher fails the step (default `false`) |
| `fuzz-for` | Approximate wall-clock budget for the whole fuzz session (default: the CLI's own, one minute) |
| `fuzz-cache` | Carry Go's fuzz cache between runs so each session resumes where the last stopped; `false` for a from-seeds run (default `true`) |
| `version` | `gomod` (default) runs the CLI from go.mod; a tag (e.g. `v1.0.0`, `latest`) installs globally |

### Outputs

| Output | Description |
|---|---|
| `exit-code` | Test process exit code |
| `coverage` | Coverage percentage (empty if coverage not enabled) |
| `badge` | Path of the rendered coverage badge SVG (empty unless badge is enabled and the tests ran) |
| `bench-report` | Path to the `gotest bench --json` report file (empty if bench not enabled) |
| `bench-breached-keys` | Comma-joined benchmark keys that breached the gate (empty if none or no gate) |
| `fuzz-crashers` | Comma-joined workspace-relative paths of the corpus files the fuzz session added (empty if none or fuzz not enabled) |

### Coverage Badge

The action can add a coverage badge to your README. `gotest` draws the badge as an SVG file and the workflow commits it to a branch of your own repository using the built-in `GITHUB_TOKEN`. Nothing outside GitHub is involved and there is no account or secret to set up.

```yaml
permissions:
  contents: write

steps:
  - uses: actions/checkout@v4
  - uses: mvrahden/go-test@v1
    with:
      packages: ./...
      min-coverage: 80
      badge: true
```

`badge: true` turns coverage reporting on by itself. The badge lands on the `ci/badges` branch (change it with `badge-branch`); embed it from there:

```markdown
![Coverage](https://raw.githubusercontent.com/<owner>/<repo>/ci/badges/coverage.svg)
```

Three rules keep the badge trustworthy:

- It is published from the default branch only. Pull requests and feature branches never change it.
- It is published only when the suites ran to completion. A build failure publishes nothing, while a failed test run or a missed `min-coverage` still publishes the coverage that run measured.
- It never affects test results. Several jobs publishing at once (a version matrix, for example) simply take turns, and a missing permission or a protected branch produces a warning, not a failed build.

Outside GitHub Actions, `gotest summary --badge=coverage.svg ./...` renders the same file for you to publish however you like.

## Commands

```bash
gotest ./... -v -race          # generate overlays and run tests (default)
gotest bench ./...             # run BenchmarkX suite methods, serially
gotest fuzz ./... --for=5m     # run FuzzX suite methods, budgeted
gotest spec ./...              # behavioral specification view
gotest summary ./...           # failure-focused summary for CI
gotest watch ./... -v          # watch mode with auto-rerun
gotest scaffold ./pkg/user.Svc # generate suite skeleton from type
gotest lint ./...              # static analysis for test suites
gotest refactor toggle-focus . # toggle F_/X_ prefixes programmatically
gotest migrate ./...           # convert testify/suite to go-test
gotest generate ./...          # run code generation only (no tests)
gotest clean ./...             # remove orphaned generated files
gotest discover ./...          # suite metadata as JSON (editor/AI surface)
gotest prepare ./tests/e2e     # start shared fixtures for debugging
gotest version                 # print version
gotest help                    # show help
```

All `go test` flags work unchanged: `-race`, `-cover`, `-count`, `-run`, `-json`, `-short`, `-timeout`, `-v`.

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
| `SuiteConfig()` method | Suite timeout/parallelism/exclusive/failfast config |
| `Hydrate` / `Dehydrate` | SharedFixture test-process resource reconstruction |

## Coding Agents

The repository ships a skill that teaches a coding agent to write suites the way gotest expects: lifecycle hooks instead of `defer`, polling instead of `time.Sleep`, the parallel recipe, condition-only `When` labels, and the version differences between releases.
It is written in the open [Agent Skills](https://agentskills.io) format, which Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot and [many other agents](https://agentskills.io/clients) load on demand.
Install it by unpacking the folder into the directory your agent reads skills from; each client documents that directory, and the client list above links to the instructions:

```bash
SKILLS_DIR=<your agent's skills directory>
mkdir -p "$SKILLS_DIR" && curl -sL https://github.com/mvrahden/go-test/archive/refs/heads/main.tar.gz \
  | tar -xz -C "$SKILLS_DIR" --strip-components=2 go-test-main/skills/writing-gotest-tests
```

> **Tip:** most agents can do this themselves. Ask yours to install the `writing-gotest-tests` skill from `github.com/mvrahden/go-test`, and it will fetch the folder and place it where it reads skills.

An agent without skill support can still be pointed at the unpacked `SKILL.md` from its instructions file, such as `AGENTS.md`. The skill source lives in [`skills/writing-gotest-tests`](skills/writing-gotest-tests).

Two commands give an agent the specification without the source. `gotest spec --static ./...` prints what a package promises before anything runs, and `gotest discover ./...` emits the same tree as JSON with a `behaviorsComplete` flag per method, so an agent never presents a partial list as the whole specification.

## VS Code Extension

The **gotest** extension brings first-class IDE support: suite-aware Test Explorer, CodeLens run/debug buttons, coverage gutters, watch mode, spec view, focus/exclude quick fixes, and suite scaffolding.
Available on the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) and [Open VSX](https://open-vsx.org/extension/mvrahden/gotest).
Install via `code --install-extension mvrahden.gotest`.

## License

MIT
