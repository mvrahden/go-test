# gotest

<p align="center">
  <img src="https://raw.githubusercontent.com/mvrahden/go-test/main/site/static/gopher.png" alt="gotest gopher" width="360" />
</p>

[![Tests](https://github.com/mvrahden/go-test/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/mvrahden/go-test/actions/workflows/test.yml?query=branch%3Amain)
[![Quality](https://github.com/mvrahden/go-test/actions/workflows/quality.yml/badge.svg?branch=main)](https://github.com/mvrahden/go-test/actions/workflows/quality.yml?query=branch%3Amain)
[![Coverage](https://raw.githubusercontent.com/mvrahden/go-test/ci/badges/coverage.svg)](https://github.com/mvrahden/go-test/actions/workflows/test.yml?query=branch%3Amain)
[![Go Reference](https://pkg.go.dev/badge/github.com/mvrahden/go-test.svg)](https://pkg.go.dev/github.com/mvrahden/go-test)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Specification-driven test suites for Go with isolation and parallelism as first-class citizens.

Write test suites as Go structs.
`gotest` generates the test functions that hand them to its runtime, and runs each suite in a process of its own.
The compiler, the `testing` package and the output are `go test`'s; the runner is `gotest`.
What you read back is a behavioral specification.

The code your tests link imports only the standard library. Suites are found and registered by code generation, never by reflection.

## Contents

- [Why gotest?](#why-gotest)
- [How It Works](#how-it-works)
- [What It Costs](#what-it-costs)
- [Install](#install)
- [Getting Started](#getting-started)
- [Specification](#specification)
- [Isolation](#isolation)
- [Parallelism](#parallelism)
- [Testing Toolkit](#testing-toolkit)
- [Benchmarking](#benchmarking)
- [Fuzzing](#fuzzing)
- [Configuration](#configuration)
- [Test Selection](#test-selection)
- [Tooling](#tooling)
- [GitHub Actions](#github-actions)
- [Commands](#commands)
- [Naming Conventions](#naming-conventions)
- [Coding Agents](#coding-agents)
- [VS Code Extension](#vs-code-extension)
- [License](#license)

## Why gotest?

Go's `testing` package gives you `func TestX(t *testing.T)` and nothing more.
Setup/teardown logic is copy-pasted or buried in `TestMain`.
As test suites grow, organization becomes a discipline problem rather than a tooling one.

**testify/suite** solves organization but adds runtime reflection, interface dispatch, and a `suite.Run(t, new(MySuite))` ceremony in every file.
Test output is standard — but the mechanism behind it isn't.

**gotest** takes a different approach:

- **Specification-driven.** BDD vocabulary (`When`/`It`) structures tests as readable behavioral contracts. `gotest spec` renders them as documentation — in the terminal, as markdown, or as structured JSON. Always in sync, never stale.
- **Isolated by default.** Each suite runs in its own process, so suites share no memory, no package-level state and no `init` side effects. Within a suite the methods share one struct value: `BeforeEach` gives each method fresh state, and a field one method writes is there for the next unless a hook resets it.
- **Parallel on two levels.** Suites run in parallel automatically, as separate processes. Methods run in parallel when the suite opts in, and each then gets its own state from a `BeforeEach` that returns it. The generator refuses a parallel suite whose `BeforeEach` returns nothing; it cannot see a method that writes a suite field, so run parallel suites under `-race`.

Under the hood, `gotest` generates one `func Test<Suite>(t *testing.T)` per suite: a table of the suite's hooks and methods, handed to the `pkg/gotestruntime` library, which runs each method as a subtest, keeps setup and teardown in order and holds the timeouts.
Generated files never touch your source tree: they reach the compiler through a build overlay and are removed after the run.

gotest's own behavioral promises are stated once in [`specs/`](specs/README.md), each scenario bound to a test; this README explains them and links there.

## How It Works

```
you write:          gotest generates:              gotest runs:
                    (in a build overlay)

MySuite struct      gotest_psuite_test.go          one process per suite:
  BeforeAll()   →     func TestMySuite(t)      →     BeforeAll()
  TestFoo()             OpenSuite(hooks)             t.Run("TestFoo", ...)
  AfterAll()            RunMethods(methods)          AfterAll()
```

The generated file is short: a wrapper type and one call each to `gotestruntime.OpenSuite` and `gotestruntime.RunMethods`, with your hooks and methods passed as plain function values. `gotest generate ./...` writes it to disk when you want to read it.
Suites are found by reading the source, and the generated code calls your methods directly: no reflection finds or dispatches a test. Assertions such as `Equal` use `reflect` to compare values.

## What It Costs

gotest puts a generator and a runner in front of `go test`. That buys the lifecycle, the fixtures and the specification view, and it has a price:

- **Two commands for a complete run.** Plain `go test` does not see suites, and `gotest` does not run plain `func Test*` functions. A module with both kinds needs both commands, each with its own coverage profile. [Install](#install) says what each prints.
- **No result cache.** gotest has no result cache. Every run executes every suite it selects; it never prints `(cached)`. Go's build cache and gotest's own cache of test binaries mean an unchanged package costs no compile and no link, but its tests run again, where `go test` would skip them. To rerun only what a change touches, use `gotest watch`: it reruns the changed package and the packages that import it ([Every run executes its tests](specs/cli/spec.md#requirement-every-run-executes-its-tests)).
- **`TestMain` runs once per suite.** Each suite is a process of its own, so a package's `TestMain`, its `init` functions and its package-level variables are set up once per suite, not once per package. Move resource setup from `TestMain` into a fixture.
- **Editors see suites through the extension only.** The [VS Code extension](#vs-code-extension) lists, runs and debugs suites. gopls and other editors show suite methods as plain methods. To debug elsewhere, write the generated code to disk first: `gotest generate ./...`, then `dlv test`, then `gotest clean ./...`.
- **Go 1.26 or newer.** The Go version follows gotest's own `go.mod`, so upgrading gotest can raise your module's `go` line; Install shows how to keep it.
- **The tool directive adds indirect requirements.** `go get -tool` records the CLI's dependencies in your `go.mod` as `// indirect`. Your test binaries link none of them: the runtime imports only the standard library.
- **Leaving means rewriting the entry points.** Assertions, `When`/`It` and `Each` are a library and keep working under plain `go test`. Suites need one hand-written `func Test*` each, and shared fixtures have no standard-library equivalent.

## Install

Add gotest to your module as a Go tool. Its version and its Go toolchain then follow your `go.mod`:

```bash
go get -tool github.com/mvrahden/go-test/cmd/gotest@latest
go tool gotest ./...
```

`go get -tool` pins the CLI and the `pkg/gotest` runtime to the same release, and `go tool` rebuilds the CLI with your module's Go. Upgrading is the same command again. What each release changed is in its [GitHub release](https://github.com/mvrahden/go-test/releases). One caution for both: `@latest` raises your module's `go` line when the newest release needs a newer Go (`go get` prints `upgraded go …`). To keep your Go version, name the newest release whose go line fits instead; `go list -m -f '{{.GoVersion}}' github.com/mvrahden/go-test@<version>` prints a release's. In a module that already requires gotest, name that version in place of `@latest` to declare the tool without moving the pin. Avoid a global `go install` binary: it drifts from `go.mod` on both axes. A CLI built with an older Go than the module's go line, or pinned below the oldest runtime it generates for, refuses to run and names the version your module selects; one drifted the other way still runs, unless it predates v1.38 and the runtime does not: then each suite fails as it starts, naming both versions and the fix.

The examples below write `gotest` for brevity; with the tool directive, read `go tool gotest`. The VS Code extension and the GitHub action run the same CLI your `go.mod` selects.

**Two runners.** `go tool gotest` runs suites and nothing else; `go test` runs plain `func Test*` functions and nothing else. Under plain `go test` a package of suites prints `ok … [no tests to run]` and exits 0, and `gotest` passes over the `func Test*` functions beside a suite with one note on stderr (`note: N stdlib test(s) in … not run`). A complete run is both commands, so every place that calls `go test ./...` today — CI, a Makefile, a pre-commit hook, an editor task — needs `go tool gotest ./...` beside it for as long as the module holds both kinds of test.

## Getting Started

Three steps. Each is useful alone, and the first needs no new command.

### Step 1: Use the Library in the Tests You Have

`pkg/gotest` is an ordinary library. In a plain `func Test*`, under plain `go test`, it gives you the assertions, `When`/`It` nesting, table rows, polling and snapshots:

```go
// price_test.go
package price

import (
    "testing"

    "github.com/mvrahden/go-test/pkg/gotest"
)

func TestTotal(t *testing.T) {
    gt := gotest.NewT(t)

    gt.When("the cart is empty", func(w *gotest.T) {
        w.It("totals zero", func(it *gotest.T) {
            gotest.Equal(it, 0, Total(nil))
        })
    })

    for it, tc := range gotest.Each(gt, []struct {
        Desc  string
        Items []int
        Want  int
    }{
        {Desc: "one item", Items: []int{5}, Want: 5},
        {Desc: "two items", Items: []int{5, 7}, Want: 12},
    }) {
        gotest.Equal(it, tc.Want, Total(tc.Items))
    }
}
```

```bash
go get github.com/mvrahden/go-test
go test ./...
```

Assertions take the `*testing.T` directly as well (`gotest.Equal(t, want, got)`), and a failure inside a helper names the helper's line and the test's without `t.Helper()`. The same tests render as a specification from their event stream, once the CLI is declared as a tool ([Install](#install)): `go test -json ./... | go tool gotest spec --input=-`.

This step changes nothing about how your tests run. Take the next one when you want lifecycle hooks, fixtures or a process per suite.

### Step 2: Write a Suite

A suite is a struct whose name ends in `TestSuite`. It runs under `go tool gotest` ([Install](#install)), not under `go test`.

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
        err := s.svc.Create("bob@example.com")
        gotest.NoError(w, err)

        w.It("returns ErrDuplicate", func(it *gotest.T) {
            err := s.svc.Create("bob@example.com")
            gotest.ErrorIs(it, err, ErrDuplicate)
        })
    })
}
```

```go
// user.go
package user

import "errors"

// ErrDuplicate reports an email that is already registered.
var ErrDuplicate = errors.New("duplicate user")

// UserService registers users by email.
type UserService struct{ users map[string]bool }

// NewUserService returns an empty service.
func NewUserService() *UserService { return &UserService{users: map[string]bool{}} }

// Create registers an email once.
func (u *UserService) Create(email string) error {
    if u.users[email] {
        return ErrDuplicate
    }
    u.users[email] = true
    return nil
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
=== RUN   TestUserServiceTestSuite/TestCreate/email_already_exists
=== RUN   TestUserServiceTestSuite/TestCreate/email_already_exists/returns_ErrDuplicate
--- PASS: TestUserServiceTestSuite (0.00s)
    --- PASS: TestUserServiceTestSuite/TestCreate (0.00s)
        --- PASS: TestUserServiceTestSuite/TestCreate/creates_a_user_with_valid_input (0.00s)
        --- PASS: TestUserServiceTestSuite/TestCreate/email_already_exists (0.00s)
            --- PASS: TestUserServiceTestSuite/TestCreate/email_already_exists/returns_ErrDuplicate (0.00s)
PASS
```

The same suites render as a behavioral specification:

```bash
gotest spec ./pkg/user
```

```
=== pkg/user — 2 passed ===

UserService — 2 passed (1ms)
  Create (<1ms)
    ✓ creates a user with valid input
    when email already exists
      ✓ returns ErrDuplicate

1 suite, 2 behaviors: 2 passed
```

No generated code leaks into your workflow.
`gotest` creates it before tests run - invisible to the project's filetree.

### Step 3: Share Setup With a Fixture

Setup that several suites need goes into a fixture: a struct ending in `Fixture` for one suite at a time, `SharedFixture` for one instance across packages. [Fixtures](#fixtures) describes both, and [`examples/sharedfixture`](examples/sharedfixture) is a shared fixture two packages bind, runnable as it stands.

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
=== pkg/user — 4 passed, 1 skipped ===

UserService — 4 passed, 1 skipped (<1ms)
  Create (<1ms)
    when email is valid
      ✓ creates the user
      ✓ sends a welcome email
    when email already exists
      ✓ returns ErrDuplicate
  Delete (<1ms)
    ✓ soft-deletes the user
    – hard-deletes after 30 days — SKIPPED: needs a clock

1 suite, 5 behaviors: 4 passed, 1 skipped
```

A skipped row carries the reason its skip gave ([Why a row was skipped](specs/spec/spec.md#requirement-why-a-row-was-skipped)).
Suite and method lines show their duration however short; a behavior or a group shows its own once it reaches a second, and a row without a verdict shows none ([A terminal report a person reads](specs/spec/spec.md#requirement-a-terminal-report-a-person-reads)). A duration is the wall clock the row occupied, never the sum of the rows beneath it — so a row that exceeds its children is time it held itself, and one that falls short of their total is children that overlapped.
Under `-count` each run of a suite is listed as a suite of its own, marked with its run ([A suite run more than once](specs/spec/spec.md#requirement-a-suite-run-more-than-once)).

Generate a markdown specification document ([A report a person reads](specs/spec/spec.md#requirement-a-report-a-person-reads)):

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

Lay a run's verdicts over that specification with `--declared`, the run's own or, with `--input`, a saved one's: every behavior the source declares is listed, and what the run did not report shows without a verdict and is counted as not run ([The declared specification under a run's verdicts](specs/spec/spec.md#requirement-the-declared-specification-under-a-runs-verdicts)). `--hide=pass` (or `fail`, `skip`, `none`, comma-separated) leaves rows out of the rendering and none out of the counts ([Hiding rows by status](specs/spec/spec.md#requirement-hiding-rows-by-status)):

```bash
gotest spec --declared ./...
gotest spec --input=events.json --declared ./...
gotest spec --input=events.json --hide=pass
```

### Scenarios from OpenSpec

A repository that states its behavior as [OpenSpec](https://github.com/Fission-AI/OpenSpec) scenarios binds them to its suites by text: a scenario's WHEN is a `When`'s label and each of its THEN and AND clauses an `It`'s label directly under it, with no marker in the test and no file to commit. `gotest scenarios` reads `specs/` and `openspec/specs/`, reports every scenario no suite states and every clause two tests could claim, and with `--verify` or a saved stream judges each bound scenario verified, failing, not run or vacuous ([Binding by text](specs/scenarios/spec.md#requirement-binding-by-text), [Ambiguity](specs/scenarios/spec.md#requirement-ambiguity), [Evidence](specs/scenarios/spec.md#requirement-evidence)):

```bash
gotest scenarios                          # which scenarios the suites state, read from source
gotest scenarios --verify --format=md     # run what is bound; a table per capability for a pull request
gotest scenarios --input=events.json --fail-on=unbound,failing,ambiguous
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
A package fixture lives as long as the suite that binds it: set up before the suite's `BeforeAll`, torn down after its `AfterAll`.
Suites that bind the same package fixture each get their own; setup that must happen once per run belongs in a shared fixture.
A suite sets up only the fixtures it binds and their dependencies, so a fixture it does not use never delays or fails it.

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
    if f.Pool != nil { // AfterAll also runs after a BeforeAll that failed
        f.Pool.Close()
    }
    return nil
}
```

Fixture hooks receive `context.Context` and return `error` — the generated wrapper reports failures with automatic attribution (e.g., `E2ESetupFixture.BeforeAll: start postgres: connection refused`).

`BeforeAll`/`AfterAll` receive `context.Background()` bounded by the fixture's configured timeout.
`BeforeEach` receives the test's `t.Context()`; `AfterEach` receives a context that a stop does not end and that ends only at a `Timeout` the suite declares, so cleanup still runs after the test's own context is cancelled ([The context a fixture's AfterEach gets](specs/fixtures/spec.md#requirement-the-context-a-fixtures-aftereach-gets)).

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
`BeforeAll`/`AfterAll` run once for each suite that binds the fixture, around that suite's own `BeforeAll` and `AfterAll`.
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
`BeforeEach`/`AfterEach` failures appear in test output — attributed to the fixture and the failing hook, and located at the hook's declaration:

```
--- FAIL: TestBatchTestSuite/TestDispatch
    fixture.go:23: E2ESetupFixture.BeforeEach failed: connection refused
```

A fixture's `AfterEach` runs only when every fixture `BeforeEach` up to its own in dependency order returned without an error.
A `BeforeEach` that returns an error gets no `AfterEach`, so it releases what it acquired before returning.

`BeforeAll`/`AfterAll` failures are fatal — they fail all fixture-bound tests, located at the hook's declaration as well:

```
--- FAIL: TestBatchTestSuite (0.01s)
    fixture.go:12: fixture setup: E2ESetupFixture.BeforeAll: start postgres: connection refused
E2ESetupFixture.BeforeAll attempt 1/2 failed: start postgres: connection refused
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
    cfg := gotest.DefaultSuiteConfig()
    cfg.Parallel = true
    return cfg
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
The runner starts `GOMAXPROCS` suite processes at once (one per core) and distributes the remaining budget as `-test.parallel` to each subprocess; `-p N` sets the number, above `GOMAXPROCS` too, for suites that wait rather than compute ([Suite processes at once](specs/cli/spec.md#requirement-suite-processes-at-once)).
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
| `-parallel M` | `min(S, GOMAXPROCS, 2 × GOMAXPROCS / M)` | `M` | `≤ 2 × GOMAXPROCS` |
| `--parallel N -parallel M` | `min(S, GOMAXPROCS, N/M)` | `M` | `~N` |
| `-p P` | `min(S, P)` | `budget / inter` | `2 × P` |

Where *S* is the number of suites and *inter* is the computed process count; beside `--parallel` or `-parallel`, `P` takes the place of `GOMAXPROCS` in the rows above.
`-p P` keeps the meaning it has for go test, the number of programs run at once: it sets the suite processes, without the halving under a sanitizer, and caps the compile parallelism. `-exec <cmd>` runs every suite binary under the command, as go test does.
Under `-race`/`-msan`/`-asan` the default budget halves (as does compile concurrency) — instrumentation at least doubles the CPU cost per instruction stream, and keeping the uninstrumented defaults would oversubscribe the machine. An explicit `--parallel`/`--compile-parallel` always wins over this heuristic.

## Testing Toolkit

### Type-Safe Assertions

Functional API with compile-time type safety:

```go
// Equality
gotest.Equal(t, expected, actual)            // [V any] — deep equality; cross-type = compile error
gotest.NotEqual(t, expected, actual)         // [V any] — deep inequality
gotest.Same(t, expected, actual)             // [V any] on *V — pointer identity
gotest.NotSame(t, expected, actual)          // [V any] on *V — distinct pointers

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
gotest.Empty(t, object)                      // nil, or len == 0; a pointer is followed; any other type fails the type guard
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

// Snapshots
gotest.MatchSnapshot(t, value)               // compares with testdata/__snapshots__/<TestName>.snap

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

All assertions work with `*gotest.T` (suites), `*testing.T` (standalone tests), `*gotest.B` and `*gotest.F` (benchmarks and fuzz targets), and `*gotest.R` (polling callbacks).

A test that passes having made no assertion proves nothing. Every leaf counts its assertions ([Counting assertions](specs/assertions/spec.md#requirement-counting-assertions)): the spec marks one that passed with none as vacuous ([A pass that asserted nothing](specs/spec/spec.md#requirement-a-pass-that-asserted-nothing)), the run names it, and `--ci` fails the run ([Passes without an assertion under CI mode](specs/cli/spec.md#requirement-passes-without-an-assertion-under-ci-mode)). A test that checks by other means says so, and the reason shows beside it ([Declaring how a test asserts](specs/assertions/spec.md#requirement-declaring-how-a-test-asserts)):

```go
t.AssertsBy("gomock's controller, given t.T(), fails the test")
```

Assertion failures automatically trace back to the test call site — no `t.Helper()` calls needed in helper functions. The failure is located at the line that failed, a helper's included, and the message names the test's call site after it.

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

Each entry becomes a subtest with its own verdict. A failing assertion on a row's `t` fails the row and the method, and the row and the rows after it still run, so one run reports every failing row. A row that calls `FailNow` or skips ends the method there ([Each rows](specs/lifecycle/spec.md#requirement-each-rows)); a row to pass over uses `continue`. Over a literal table, `gotest spec --declared` lists the rows a stopped row left unrun, as not run.
Uses `Desc` or `Name` field for the test name, falls back to `#0`, `#1`, etc.

### Testing a Callback

For an API that reports through a callback, close a channel from the callback and wait for it in the method, asserting on the test goroutine:

```go
func (s *BusTestSuite) TestDelivery(t *gotest.T) {
    got := make(chan Event, 1)
    s.bus.Subscribe(func(evt Event) { got <- evt })
    s.bus.Publish(Event{Kind: "created"})
    select {
    case evt := <-got:
        gotest.Equal(t, "created", evt.Kind)
    case <-t.Context().Done():
        gotest.Fail(t, "no event delivered")
    }
}
```

For a condition that is polled rather than signalled, use `Eventually`.

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

For snapshots taken in plain `func Test*` tests, which run under `go test`:

```bash
GOTEST_UPDATE_SNAPSHOTS=1 go test ./...
```

Suites do not run under `go test`, so this command leaves their snapshots as they are.

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
`BeforeEach`/`AfterEach` run once per benchmark method, outside the timing window — the generated wrapper stops the timer before `BeforeEach`, starts and resets it right before your method runs, and stops it again before `AfterEach`, which runs even when the benchmark fails or panics.
Use `b.Loop()` (Go 1.24+) rather than `b.N` — it excludes setup before the loop from timing automatically.

The suite must be named `*TestSuite`, even if it only has benchmarks — a struct without that suffix is no suite, and gotest refuses the run when a test file declares `Benchmark*` methods taking `*gotest.B` on it that no test file calls ([Cases no run reaches](specs/discovery/spec.md#requirement-cases-no-run-reaches)).
Benchmarks can't coexist with a returning `BeforeEach` (its context type can't thread through `*gotest.B`), and every lifecycle hook on a benchmark suite must take `*gotest.T`, not `*testing.T`.
Fixture-bound suites work — fixtures hydrate before benchmarks run — but a fixture with `BeforeEach`/`AfterEach` bound to a suite that has `Benchmark*` methods is rejected at generation time: per-method fixture hooks aren't supported for benchmarks.

### Running

```bash
gotest bench ./...                                        # all benchmarks
gotest bench ./pkg/parser -run Parser                     # suites whose name matches
gotest bench ./pkg/parser -bench Parser                   # the same, via -bench
gotest bench ./pkg/parser -bench 'ParserTestSuite/Parse'  # one suite's methods matching Parse
```

`gotest ./...` never runs benchmarks — use `gotest bench` explicitly.

Benchmark suites run serially, one suite process at a time.
Running benchmarks concurrently makes their timing numbers meaningless — the runner disables streaming and compiles everything up front so `go test -c` never competes with a running benchmark for CPU.
A fresh process per suite is also a methodological win, not just an implementation detail: GC pressure from one benchmark can't pollute another's numbers.

`-test.benchmem` is on by default — every benchmark line reports `B/op`/`allocs/op` alongside `ns/op`.
`-benchtime` and `-count` are forwarded to `go test` unchanged.
`-coverprofile` works; `--min` (coverage gate) isn't available in bench mode.
`-run` and `-bench` both filter which suites run, both matched against each suite's `Benchmark<SuiteName>` wrapper function by the first slash segment of each alternative, as go test matches a top-level name; `-skip` drops a suite whose wrapper it skips whole, as go test does. A plain `-bench Parse` therefore selects suites, not methods; later segments select the methods inside a suite's wrapper (`-bench 'ParserTestSuite/BenchmarkParse'`).
Given both, a suite must match both to run (AND semantics).
`-run Test<Suite>`-style values match nothing in bench mode — filter by the suite name itself, not the `Test` prefix.

With no matching benchmarks, `gotest bench` prints `no benchmarks found` and exits 0; patterns that match no package at all exit 2, as in every run mode.

### Spec view

```bash
gotest bench --spec ./examples/notification -benchtime=10x
```

```
=== examples/notification ===

BenchmarkNotificationDispatchBench (<1ms)
  ✓ Dispatch  1103 ns/op · 569 B/op · 2 allocs/op

1 suite, 1 benchmark
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
They pair as a test method's do: a failing or panicking `BeforeEach` still runs its `AfterEach` ([Execution hooks pair](specs/fuzz/spec.md#requirement-execution-hooks-pair)).

Because `f.Fuzz` takes `any`, the callback's shape is checked when gotest generates, and the message names the method:

- the callback must take a leading `*gotest.T` and return nothing;
- `f.Fuzz` may be called once per method;
- `*testing.F` is rejected, since the per-execution lifecycle needs `gotest.F`.

The suite-shape rules match benchmarks, with fuzz-specific messages: a returning `BeforeEach` is rejected (move fuzz targets to a dedicated suite), every lifecycle hook must take `*gotest.T`, and a fixture with `BeforeEach`/`AfterEach` cannot bind to a fuzzing suite (per-execution fixture hooks are not supported).

Each `Fuzz*` method becomes one top-level function named `Fuzz<SuiteIdentifier>_<MethodName>`, e.g. `FuzzParserTestSuite_FuzzParse`, because Go's fuzzing engine targets one `FuzzX` symbol per run.

### Running

```bash
gotest fuzz ./...                     # one minute, shared across all targets
gotest fuzz --for=5m ./...            # about five minutes across all targets
gotest fuzz --for=1m --jobs=2 ./...   # two targets at a time
gotest fuzz --target=FuzzParserTestSuite_FuzzParse ./pkg/parser
```

`gotest ./...` replays every seed as an ordinary subtest; `gotest fuzz` searches for new inputs under a time budget and exits 1 when it finds a crasher. Struct arguments, seed harvesting, the session budget, triage and promote, and what plain `go test` does not see are in [docs/fuzzing.md](docs/fuzzing.md).

## Configuration

Every fixture and suite runs with defaults: a fixture's setup has two minutes, and a test's `t.Context()` ends after 30 seconds. A default only ends the context; a test is failed for overrunning only a `Timeout` its suite declares ([A context the default deadline ended](specs/lifecycle/spec.md#requirement-a-context-the-default-deadline-ended)).
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

A duration the returned config leaves at zero (omitted or explicit) gets the default, exactly as if the marker were absent, so `gotest.SuiteConfig{Parallel: true}` runs with the 30-second timeouts.
`gotest.NoDeadline` (or any negative duration) disables a deadline; booleans, `Retries` and `RetryDelay` are used as written.
Start from a preset to override its durations (`cfg := gotest.IntegrationSuiteConfig(); cfg.Parallel = true; return cfg`).

`Exclusive: true` schedules the suite's process strictly alone: after every non-exclusive suite has finished, one exclusive suite at a time, in deterministic order.
Use it for suites whose verdicts depend on wall-clock behavior or contended resources (timing budgets, containers, ports, heavy child builds) — a budget verdict taken on a saturated machine is not a verdict you can act on.
Like `Parallel`, it is resolved statically by the generator and affects scheduling only.
Two runs of one module on one machine, such as two agents in sibling worktrees, take turns at their Exclusive suites and at each named group; a run that has to wait says so once on stderr, and `GOTEST_EXCLUSIVE_LOCK=0` turns this off ([Exclusive across runs](specs/lifecycle/spec.md#requirement-exclusive-across-runs)).

`ExclusiveGroup: "e2e"` puts the suite in a named group that runs apart from every other suite, its members in parallel among themselves: the default group runs first, then each named group by name, and `Exclusive` then means alone within the group ([Exclusive groups](specs/lifecycle/spec.md#requirement-exclusive-groups)).
A shared fixture only the group reads starts when the group starts and is released at its last use; a group that runs first, such as the one `--group` selects, starts it beside compilation, as the default group does.
The value must be a string literal or a string constant, possibly from another package (`testgroups.E2E`), using `A-Z`, `a-z`, `0-9`, `_` and `-`.
`gotest ./... --skip-group=e2e` and `gotest ./... --group=e2e` split a run along the same lines, for example into two CI jobs ([Selecting groups](specs/cli/spec.md#requirement-selecting-groups)); `gotest summary` prints each group's suites and wall time.
Group only what needs isolation: a grouped suite gives up the overlap it had with the rest of the run.

Preset constructors for common scenarios:

| Preset | Timeout | SetupTimeout | Retries | RetryDelay | Use case |
|--------|---------|--------------|---------|------------|----------|
| `DefaultFixtureConfig()` | 2 min | — | 0 | — | Standard fixtures |
| `ContainerFixtureConfig()` | 5 min | — | 1 | 5 sec | Testcontainers, image pulls |
| `DefaultSuiteConfig()` | 30 sec | 30 sec | — | — | Unit/integration tests |
| `IntegrationSuiteConfig()` | 2 min | 5 min | — | — | Heavier integration tests |

### Project Configuration

Project-level defaults live in `.gotest.yml` at the repository root (or nearest parent with a `go.mod`).
CLI flags always take precedence over config values, and a value its flag would refuse fails the run with exit 2, naming the key ([A broken config](specs/cli/spec.md#requirement-a-broken-config)); omitted keys fall back to defaults (an explicit `0` on a duration key disables the deadline — a flag or a key can be absent, so `0` is free to mean off; a `SuiteConfig` field cannot, so there `0` means the default and `gotest.NoDeadline` means off).

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
| `timeout` | duration | `--timeout` | Global pipeline deadline (default: 15m, 0 to disable); `gotest fuzz` ignores it and `setup-timeout`, since `--for` is its clock ([Config defaults a command cannot take](specs/cli/spec.md#requirement-config-defaults-a-command-cannot-take)) |
| `setup-timeout` | duration | `--setup-timeout` | Flat budget for shared fixture setup (default: what the fixtures declare — each one's `Timeout × (Retries+1) + RetryDelay × Retries`, plus `Timeout × Retries` for the releases between attempts when it has an `AfterAll`, 30s until the first reports; 0 to disable; see [the setup-budget requirement](specs/fixtures/spec.md#requirement-a-shared-fixtures-declared-setup-budget)) |
| `min-coverage` | int | `--min` | Minimum coverage percentage (0–100) |
| `parallel` | int | `--parallel` | Total concurrent test method budget |
| `compile-parallel` | int | `--compile-parallel` | Concurrent compilation processes (default: NumCPU, auto-halved for -race/-msan/-asan) |
| `debounce` | duration | `--debounce` | Watch mode re-run delay |
| `lint.skip` | list | — | Non-integrity lint rules to disable project-wide |
| `lint.nolint-allow` | list | `-nolint-allow` | Linter names a `//nolint` may name beside golangci-lint's |
| `bench.baseline` | path | `--against`, `--save=` | Baseline `gotest bench` compares against and a bare `--save=` writes |
| `bench.gate` | float | `--gate` | Regression percentage that fails a bench run (0 disables) |
| `fuzz.harvest` | bool | `--no-harvest` | Seed harvesting from table tests (default: true) |
| `scenarios.specs` | list | `--specs` | Spec files and directories `gotest scenarios` reads, relative to the module root (default: `specs`, `openspec/specs`) |

## Test Selection

### Focus and Exclude

```go
type F_UserServiceTestSuite struct { ... }  // F_ prefix: only this suite runs
type X_BrokenTestSuite struct { ... }       // X_ prefix: this suite is skipped

func (s *MySuite) F_TestCreate(t *gotest.T) {} // focus a single test
func (s *MySuite) X_TestFlaky(t *gotest.T)  {} // exclude a single test
```

Focus spans the whole run: a focused suite or method in any package skips every unfocused suite in every package of the run ([Focus across a run](specs/discovery/spec.md#requirement-focus-across-a-run)).
Outside CI mode a focused run, `gotest spec` and `gotest summary` say so on stderr before anything runs, with how many items carry `F_` and where the first is declared ([A focused run outside CI mode](specs/cli/spec.md#requirement-a-focused-run-outside-ci-mode)).

Use `--ci` in CI to fail the build if any `F_` prefix slipped through or a test passed without an assertion, and to enable snapshot read-only mode (missing baselines fail instead of being generated). Every output mode judges it, a text run without `-v` too ([Passes without an assertion under CI mode](specs/cli/spec.md#requirement-passes-without-an-assertion-under-ci-mode)):

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
The guard runs before the suite's fixtures, config, `BeforeAll` and every test method, so a skipped suite sets up none of its package fixtures; its fixture fields are still nil inside the guard.
A shared fixture the suite binds is still started by the runner, which cannot evaluate the guard.
Unlike `X_` (static exclude), `SuiteGuard` makes the decision at runtime — useful for integration tests that need external services.

## Tooling

### Watch Mode

Re-run tests on file changes with 200ms debounce:

```bash
gotest watch ./... -v
gotest watch ./... --spec     # watch + spec view
```

The changed package and the packages that depend on it are re-run.
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

Renames lifecycle methods, rewrites assertions (picking, by the argument's type, the gotest assertion that keeps testify's verdict), removes the testify imports nothing uses any more. The result is type-checked for real, in every build its `//go:build` tags need: anything it cannot convert, and every line that does not compile, gets a `TODO(gotest-migrate)` marker, and every run exits 1 while one is left. Exit 0 means every migrated package compiles and every hook and test testify ran is one gotest calls; a package that does not compile before is left alone, exit 2, as is a run whose files cannot all be written (it writes them all or none). Files headed `// Code generated ... DO NOT EDIT.` are skipped (named on stderr), and only the host OS's builds are checked: a test file only another GOOS builds is refused, so run migrate on each platform whose test files differ. The module must require `github.com/mvrahden/go-test`; `--dry-run` prints the diff without writing.

Where a one-to-one translation would change what a test passes or fails on, or what runs when, migrate leaves the code under a marker that gives the reason and the gotest way ([Kept verdicts](specs/migrate/spec.md#requirement-kept-verdicts)). A suite built on a base suite of the same test files migrates with it, the base becoming a helper the child calls, `BaseTestSuite` renamed `BaseSuiteHelper` ([Base suites](specs/migrate/spec.md#requirement-base-suites)). Migrate never writes `go.mod` or `go.sum` ([Module files](specs/migrate/spec.md#requirement-module-files)), and leaves no blank line where testify imports stood ([Imports](specs/migrate/spec.md#requirement-imports)).

### Linter

Catch common mistakes in test suites with static analysis:

```bash
gotest lint ./...
```

Thirty-five rules in three tiers:

- **Integrity** — violations can make test outcomes unreliable or leak resources: committed `F_` prefixes, value receivers on suite methods, lifecycle hook typos, `BeforeAll` without `AfterAll`, `X_` prefixes on lifecycle hooks, wrong test signatures, suite-lifecycle bypasses via `t.T()` (`Cleanup`/`Parallel`/`Run`) and via `b.B()` or `f.F()` (`Cleanup`), outer `t` inside `Eventually`/`Consistently` callbacks, `Nil`/`Empty` assertions on types their runtime guards reject, reads of shared fixtures a suite never declared (window scheduling only starts what is declared), suites and fixtures declared inside a `type ( … )` block (the fix gives each its own declaration), embedded suites, fixtures or hook-carrying types and suite-typed fields, direct calls of a lifecycle hook the harness already calls, `Test`, `Benchmark` and `Fuzz` methods no suite runs (on a type without the `TestSuite` suffix, named just `Test`, or on a generic suite without an alias; [Cases that never run](specs/lint/spec.md#requirement-cases-that-never-run)), every other shape the generator rejects, `//nolint` comments naming no rule a linter knows or giving no reason, and generated files checked into version control.
- **Expressiveness** — the test is correct but its syntax can be improved: simplifiable assertions (`True(t, a == b)` → `Equal`, `Len(t, x, 0)` → `Empty`, …), redundant assertions, `if cond { Fail(...) }` guards that an assertion expresses directly, unnecessary `t.T()`, `b.B()` and `f.F()` escapes, `When("when …")`/`It("it …")` descriptions that spell the word the spec already supplies, and a literal negative `SuiteConfig`/`FixtureConfig` timeout (every negative disables the deadline; the fix writes `gotest.NoDeadline`, keeping the old meaning). `-fix` applies the safe rewrites in one run, overlapping ones included (an assertion counts as redundant only when the arguments of both are plain names, fields, elements or literals, so removing it cannot skip a call), then reports what it could not fix (exit 3 while findings remain); `-diff` prints exactly what `-fix` would write, as a unified diff with paths relative to the current directory, and writes nothing — `gotest lint -diff ./... | git apply` applies it (a file outside that directory needs `git apply --unsafe-paths`; a symlink is named by the file it leads to, which `-fix` writes).
- **Migration** — adoption aids for codebases moving to gotest: stdlib test functions and testify imports; coexistence is legitimate.

`gotest lint -rules` lists them, and `gotest lint -rules -json` gives each rule's tier, scope, whether it can be skipped or fixed, and its description as a document an editor or a script reads ([The rules](specs/lint/spec.md#requirement-the-rules)).

Each finding line ends with its rule in parentheses, `file:line:col: message (rule)`, the id `//nolint:<rule>` takes.
Suppress per line with `//nolint:<rule> // <reason>` (on the same line, or in a comment block of its own ending on the line directly above — a comment trailing other code does not reach the next line); expressiveness and migration rules can also be disabled project-wide via `.gotest.yml` (`lint.skip`). A suppression gives its reason after ` //`, and `gotest lint -suppressions` lists every one, with its reason and whether it suppressed anything, as a JSON document an auditor can diff between two trees ([A nolint comment gives its reason](specs/lint/spec.md#requirement-a-nolint-comment-gives-its-reason), [The suppression list](specs/lint/spec.md#requirement-the-suppression-list)). See the [design spec](docs/design/spec.md#linter) for the full rule table.
Inside GitHub Actions (or with `--github`), the plain-text report is replaced: findings are printed as `::error` annotations, which GitHub shows inline on the PR, and appended as a table to the step summary; `--github=false` keeps the plain report inside GitHub Actions. (The standalone `gotest-lint` binary was retired; `gotest lint` accepts the same targets and driver flags.)

`gotest lint` is the reference driver, and `pkg/lint` exports the analyzer for two others. golangci-lint runs it as a module plugin, `github.com/mvrahden/go-test/pkg/lint/golangci`, named in `.custom-gcl.yml` at the gotest version your `go.mod` requires and enabled as `gotestlint` with the settings `skip` and `nolint-allow`; `go vet -vettool` runs it through a small `main` around `unitchecker`. The [`pkg/lint` package documentation](https://pkg.go.dev/github.com/mvrahden/go-test/pkg/lint) shows both setups and what each driver leaves out.

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

By default (`version: gomod`), the action builds the `gotest` your `go.mod` selects once per job and runs it, leaving the checkout's `go.sum` as committed — no version drift between CI and local development. Set `version: latest` or a specific tag to install a standalone binary instead.

The action emits `::error` annotations that appear inline on PR diffs and writes a markdown summary to the GitHub step summary panel; the summary also notes the stdlib tests beside your suites that gotest did not run.

Inputs, outputs, the event stream, the fuzz and bench steps and the coverage badge are in [docs/github-action.md](docs/github-action.md). How a cancelled job, a container and a closed terminal stop a run is in [docs/stopping-a-run.md](docs/stopping-a-run.md).

## Commands

```bash
gotest ./... -v -race          # generate overlays and run tests (default)
gotest --failed               # rerun only what the last run here failed
gotest --changed ./...        # run only the packages whose sources changed
gotest runs                    # list the runs made here, and what is still current
gotest bench ./...             # run BenchmarkX suite methods, serially
gotest fuzz ./... --for=5m     # run FuzzX suite methods, budgeted
gotest spec ./...              # behavioral specification view
gotest summary ./...           # failure-focused summary for CI
gotest watch ./... -v          # watch mode with auto-rerun
gotest scaffold ./pkg/user.Svc # generate suite skeleton from type
gotest lint ./...              # static analysis for test suites
gotest audit --base=origin/main  # what a change did to the suites, and what got weaker
gotest audit --base=origin/main --run  # and what a run of what changed proved
gotest scenarios --verify      # OpenSpec scenarios bound to When/It by text, judged by a run
gotest refactor toggle-focus user_test.go UserTestSuite  # toggle an F_ prefix on a suite (or Suite.Method)
gotest skill install           # install the agent skill matching this version
gotest hook pre-shell          # answer a coding agent's hook event on stdin (the plugin wires it)
gotest migrate ./...           # convert testify/suite to go-test
gotest generate ./...          # run code generation only (no tests)
gotest clean ./...             # remove orphaned generated files
gotest clean --cache           # empty the test binary cache and the audit's base trees
gotest discover ./...          # suite metadata as JSON (editor/AI surface)
gotest prepare ./tests/e2e     # start shared fixtures for debugging
gotest version                 # print version
gotest help                    # show help
```

The `go test` flags keep their meaning: `-race`, `-cover`, `-count`, `-run`, `-skip`, `-list`, `-json`, `-short`, `-timeout`, `-v`, `-p`, `-exec` and the build flags. A few would break what gotest does and are refused with the reason, exit 2 ([Go test flags gotest implements](specs/cli/spec.md#requirement-go-test-flags-gotest-implements)): `-o`, `-c` and `-n`, since gotest builds the binaries itself; `-overlay`, since gotest builds through an overlay of its own; `-fuzz`, which `gotest fuzz` replaces; and `-cpuprofile`, `-memprofile`, `-blockprofile`, `-mutexprofile` and `-trace` on a run that starts more than one suite process, which would each write the one file. Every word after `-args` goes to the test binary as written.
`-run` reaches each suite process as a pattern for that suite alone ([A -run pattern in each suite process](specs/cli/spec.md#requirement-a--run-pattern-in-each-suite-process)). `-run '^TestMySuite$'` names the suite's test function only, so its fuzz seeds do not replay; name the fuzz wrapper as well, or drop the anchors, to replay them.
A filtered run builds only the packages it runs, so a build or vet failure in a package the filter leaves out is not reported by that run; `go test` builds every matched package ([Packages a run builds](specs/cli/spec.md#requirement-packages-a-run-builds)).
Test binaries are stripped of their symbol table and debug information, as `go test` strips them; `-ldflags` (an argument or in `GOFLAGS`), a profile flag such as `-cpuprofile`, or `--debug` keeps both in ([How test binaries are linked](specs/cli/spec.md#requirement-how-test-binaries-are-linked)).
`--debug` also ends a run with one line on stderr of the time each phase took, `DEBUG: phases: discover=… generate=… compile=… run=… report=… total=…`; compiling overlaps running, so `compile` is a part of `run`, not a phase after it ([Where a run's time went](specs/cli/spec.md#requirement-where-a-runs-time-went)).

A run exits `0` when everything passed, `1` on a failed test (one that panics or calls `os.Exit` included — `os.Exit(0)` too, as under `go test`), a shared fixture that failed to set up or tear down, or an expired `--timeout` (a suite it stopped fails its package even when its methods passed: [A suite cut short by --timeout](specs/cli/spec.md#requirement-a-suite-cut-short-by---timeout)), `2` on a usage, generation or build error, on a text run in which a suite reports that it ran no tests ([A suite that ran no tests](specs/cli/spec.md#requirement-a-suite-that-ran-no-tests)), on a green run in which a declared test never ran (a plain text run judges each suite by its binary's result line, and `-run`, `-skip`, `-list`, `-count=0` and `-h` after `-args` stand the census down), and when the patterns match no package, and `130` when it was interrupted. `lint` and `fuzz` state their own codes in `gotest help lint` and `gotest help fuzz`.

To feed a JUnit consumer, run the tests through a converter that reads the events: `gotestsum --junitfile report.xml --raw-command -- go tool gotest -json ./...`. The report is red whenever the run is — a package that does not build or a shared fixture that fails included — and gotest's exit code passes through. A converter that parses the printed text instead of the events, such as `go-junit-report`, reports those failures as plain output or not at all.

A `-json` run writes a suite's events when its suite process ends, all at once. `--live` (or `GOTEST_LIVE=1`) writes them as the process writes them, so a reader sees each test start and end while its suite still runs; only the suite's own events keep their order, and a package's verdict still waits for its last suite ([A live event stream](specs/cli/spec.md#requirement-a-live-event-stream)).

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
| `*Fixture` suffix | Package fixture: one instance for each suite that binds it |
| `*SharedFixture` suffix | Cross-package shared fixture |
| `FixtureConfig()` method | Fixture timeout/retry config |
| `SharedFixtureConfig()` method | Shared fixture timeout/retry config |
| `SuiteConfig()` method | Suite timeout/parallelism/exclusive/failfast config |
| `Hydrate` / `Dehydrate` | SharedFixture test-process resource reconstruction |

## Coding Agents

The repository ships a skill that teaches a coding agent to write suites the way gotest expects: lifecycle hooks instead of `defer`, polling instead of `time.Sleep`, the parallel recipe, condition-only `When` labels, and the version differences between releases.
It is written in the open [Agent Skills](https://agentskills.io) format, which Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot and [many other agents](https://agentskills.io/clients) load on demand.
The surest install is the CLI itself (v1.35.0+), because it writes the skill shipped with the gotest version your `go.mod` pins:

```bash
go tool gotest skill install                         # .agents/skills: Copilot, Codex, Cursor, Gemini CLI, Antigravity, ...
go tool gotest skill install --dir=.claude/skills    # Claude Code
```

Rerun it after upgrading gotest; it replaces the older copy.
The directory is relative to where you run it: where the Go module sits in a subdirectory of the repository, point `--dir` at the repository root's skills directory, which is where agents look.
Agents with a plugin system can install it as a plugin instead. A plugin carries the newest skill rather than the one for your pin: plugins install from the repository's main branch, and the VS Code extension bundles the skill of its release.

| Agent | Install |
|---|---|
| Claude Code | `/plugin marketplace add mvrahden/go-test`, then `/plugin install gotest@go-test` |
| Codex | `codex plugin marketplace add mvrahden/go-test`, then `codex plugin add gotest@go-test` |
| GitHub Copilot CLI | `copilot plugin marketplace add mvrahden/go-test`, then `copilot plugin install gotest@go-test` |
| VS Code with Copilot Chat | included in the [gotest extension](vscode-gotest) |
| Any other agent | `npx skills add mvrahden/go-test` |

The Claude Code plugin also carries hooks that call `gotest hook` ([An agent's hook events](specs/cli/spec.md#requirement-an-agents-hook-events)).
After the agent edits a `_test.go` file, it hands the agent what `gotest lint` reports for that package ([Lint findings after an agent's edit](specs/cli/spec.md#requirement-lint-findings-after-an-agents-edit)).
Before the agent runs `go test` over suites, which `go test` does not run, it names the gotest command line that does ([A note before go test skips the suites](specs/cli/spec.md#requirement-a-note-before-go-test-skips-the-suites)); the command still runs.
When the agent is about to end its turn in a git checkout, it audits the working tree against the merge base with `origin`'s default branch, without running a test, and hands back what the change weakened, removed or excluded tests, a guard, a raised budget, a new suppression and the like: the agent goes on to undo each or say so in its final message ([Weakenings at the end of an agent's turn](specs/cli/spec.md#requirement-weakenings-at-the-end-of-an-agents-turn)). Each weakening is handed back once per checkout, the state kept in gotest's cache; the wrapper starts no CLI in a checkout with no changed or untracked `.go` file, so a weakening only committed since the last turn waits for the next changed one.
The hooks use `go tool gotest` where `go.mod` declares the tool and the `gotest` on `PATH` otherwise, download nothing, and do nothing where neither knows the `hook` command.
They need `sh`, which Claude Code on Windows has through Git Bash.

Codex reads the same hook events from `.codex/hooks.json` (or `~/.codex/hooks.json`), which you trust once with `/hooks`. In a module that declares the tool, this gives Codex the note before `go test`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "GOPROXY=off go tool gotest hook pre-shell", "timeout": 30 }]
      }
    ]
  }
}
```

Codex's `Stop` event carries the same `cwd` and `stop_hook_active` fields and continues the turn on the same `decision: "block"` answer, so the stop audit wires the same way:

```json
{
  "hooks": {
    "Stop": [
      { "hooks": [{ "type": "command", "command": "GOPROXY=off go tool gotest hook stop", "timeout": 30 }] }
    ]
  }
}
```

Codex passes an `apply_patch` edit to its hooks in `tool_input.command`, with no file path, so `post-edit` has no file to lint there; Cursor's shell hook must return a permission decision, so it is not wired.

An agent without skill support can still be pointed at the installed `SKILL.md` from its instructions file, such as `AGENTS.md`. The skill source lives in [`skills/writing-gotest-tests`](skills/writing-gotest-tests).

Under an agent a run prints for it. Where `AI_AGENT` is set, or `CLAUDECODE=1`, a text run prints only what failed, whole, and ends stdout with one line that holds the verdict, the exit code, the counts and the command that reruns the failures, so `tail -1` cannot lose it ([Agent output](specs/cli/spec.md#requirement-agent-output), [The closing line](specs/cli/spec.md#requirement-the-closing-line)); `--agent=false` or `GOTEST_AGENT=0` turns it off, and `--agent` turns it on anywhere.
Outside agent output the same line closes stderr, after go test's text on stdout.
A suite binary that ran none of its tests, as under a `TestMain` that never calls `m.Run`, fails a text run as it fails `-json` ([The census in a text run](specs/cli/spec.md#requirement-the-census-in-a-text-run)).
Every run leaves a record of what it failed in the gotest cache, and `gotest --failed` reruns only that, by method, a pass without an assertion that failed a `--ci` run included, in a text or `-json` run, `spec` or `summary` ([Rerunning what failed](specs/cli/spec.md#requirement-rerunning-what-failed)).
Each package's verdict carries an input hash of the source it judged ([A package's input hash](specs/cli/spec.md#requirement-a-packages-input-hash)), so `gotest --changed` runs only the packages whose hash moved since the last run, or that failed in it, and says on its closing line how many it left out ([Running what changed](specs/cli/spec.md#requirement-running-what-changed)); a hash cannot see the environment or files read outside `testdata`, so a final check runs everything ([What an input hash cannot see](specs/cli/spec.md#requirement-what-an-input-hash-cannot-see)). `gotest runs` lists the runs kept for the directory with how many of their packages are still current ([Listing the runs](specs/cli/spec.md#requirement-listing-the-runs)), and `spec --input=<file> --check-fresh` marks the stale packages of a saved stream ([Checking a stream against the tree](specs/cli/spec.md#requirement-checking-a-stream-against-the-tree)).

Two commands give an agent the specification without the source. `gotest spec --static ./...` prints what a package promises before anything runs, and `gotest discover ./...` emits the same tree as JSON with a `behaviorsComplete` flag per method, so an agent never presents a partial list as the whole specification.

## VS Code Extension

The **gotest** extension brings first-class IDE support: suite-aware Test Explorer, CodeLens run/debug buttons, coverage gutters, watch mode, spec view, focus/exclude quick fixes, and suite scaffolding.
Available on the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=mvrahden.gotest) and [Open VSX](https://open-vsx.org/extension/mvrahden/gotest).
Install via `code --install-extension mvrahden.gotest`.

## License

MIT
