# gotest Specification

> **Normative.** This document is the source of truth for gotest's design.
> `AGENTS.md`, `README.md`, help texts, and the site derive from it — update this file first.
> One drift guard enforces partial sync: the CLI-surface test in `cmd/gotest`
> (subcommand/flag table membership, both directions).
> API coverage and prose semantics remain hand-verified.

Go tests that write themselves, organize themselves, and explain themselves.

`gotest` closes the gap between `func TestX(t *testing.T)` and a well-organized test suite through code generation.
Developers write structs, name them well, and the tool handles the rest.
No third-party runtime dependencies.
No reflection in suite discovery or registration.
No lock-in.
Just standard Go tests with lifecycle management and structured organization.

A Go developer should be able to `go install` this tool and immediately write better-organized tests without learning a framework.
The naming conventions are the API.
The generated code is the implementation.
The output is `go test` output.

---

## Design Principles

Ranked. When they conflict, higher-ranked principles win.

### 1. Standard Go output, always

Every generated test is a `func Test*(t *testing.T)`.
Every line of output is standard `go test` output.
Every CI system, IDE, coverage tool, and profiler works unchanged.

### 2. The naming IS the API

No config files for test semantics.
No struct tags.
No annotations.
No registration calls.
A developer reads the naming conventions once and never opens documentation again.
(The optional `.gotest.yml` holds only tool defaults — every key mirrors a CLI flag; deleting it changes no test's meaning.)

### 3. Zero runtime magic

The tool generates code at build time and cleans it up after.
No reflection, no interface dispatch, no type assertions, no registration.
At test execution time the call stack holds the thin `gotest.T` wrapper, and — for fixture-bound suites only — the small `pkg/gotestruntime` package that orchestrates fixture DAG setup/teardown.
Both are plain Go, visible in the generated code, which is what a careful developer would write by hand.

### 4. Invisible until needed

A developer who has never heard of `go-test` can read a test suite struct and understand what it does.
Plain `go test` keeps working untouched: it runs stdlib tests and ignores suites entirely — suite tests are *methods*, and the standard runner only executes top-level `func Test*` functions.
Running suites is exclusively `gotest`'s job (see The Two Runners).

### 5. Adopt incrementally, eject freely

Existing `func Test*` tests coexist with suites in the same package.
They keep running under `go test`; `gotest` reports them but does not execute them (see The Two Runners).
Removing the tool means deleting suite structs and writing the equivalent `func Test*` functions — the generated code shows exactly what to write.

---

## Conceptual Model

Test suites are behavioral specifications.
Every level of the test hierarchy maps to a specification concept:

```
struct  = Subject     "UserService"
method  = Capability  "Create"
When()  = Context     "when email is valid"
It()    = Behavior    "creates the user"
Each()  = Variants    "standard format", "missing @", "empty string"
```

The naming conventions at the struct/method level and the string descriptions at the `It`/`When` level together form a complete behavioral specification.
The tool generates the bridge code (lifecycle, parallel coordination, focus/exclude) and can render the full specification in human-readable form.

---

## The Two Runners

`go test` and `gotest` partition the test universe — each runs its half and ignores the other's:

| | stdlib `func Test*` | Suites |
|---|---|---|
| `go test` | **runs** | ignores (suite methods are not test functions) |
| `gotest` | reports, does not run | **runs** (via generated code) |

This is deliberate: `go test` stays fully usable with its caching and ecosystem, and `gotest` stays a focused suite runner rather than a `go test` wrapper.
A complete run is both commands; the canonical CI shape is two steps — `go test ./...` followed by `gotest`.

`gotest` reports the half it does not run:
packages with stdlib tests but no suites print `?   <pkg>  [no suites]` (never the false `[no test files]`), and a run that skipped stdlib tests ends with a note on stderr:

```
note: 8 stdlib test(s) in 1 package(s) not run — gotest runs suites; use 'go test' for stdlib tests
```

---

## CLI Interface

```
gotest [subcommand] [packages...] [go-test-flags...] [--gotest-flags...]
```

### Subcommands

| Command | Effect |
|---------|--------|
| *(default)* | Generate suites, run `go test`, cleanup |
| `watch` | Re-run on file changes |
| `clean` | Remove orphaned generated files |
| `generate` | Generate suite files without running tests |
| `scaffold` | Generate test suite skeleton from a Go type |
| `migrate` | Convert testify/suite tests to go-test suites |
| `spec` | Run tests and render behavioral specification |
| `summary` | Run tests and render a failure-focused summary (CI mode) |
| `bench` | Run `BenchmarkX` suite methods serially via `go test -bench` |
| `fuzz` | Orchestrate `FuzzX` suite targets with a shared time budget |
| `lint` | Run gotest-specific linter checks |
| `refactor` | Toggle focus prefixes: `refactor toggle-focus <file> <Suite[.Method]>` |
| `discover` | Discover test suites and output JSON metadata |
| `prepare` | Start shared fixtures for debug (blocks until SIGTERM) |
| `version` | Print version information |
| `help` | Show help |

### Flags

| Flag | Effect |
|------|--------|
| `--debug` | Keep generated files after run |
| `--ci` | CI mode: fail on `F_` prefixes, snapshot read-only |
| `--no-cache` | Bypass the overlay write cache (generation itself is always fresh) |
| `--spec` | Render the spec view instead of the default output (default and watch modes) |
| `--update-snapshots` | Regenerate snapshot files |
| `--format=<fmt>` | Output format for `spec`/`summary` (terminal, md/markdown, json) |
| `--output=<path>` | Write formatted output to file |
| `--no-color` | Strip ANSI codes from terminal output |
| `--min=<pct>` | Fail if coverage below threshold (enables `-coverprofile`) |
| `--setup-timeout=<dur>` | Total budget for shared fixture setup (default: 2m; 0 disables) |
| `--timeout=<dur>` | Global pipeline deadline (default: 15m; 0 disables) |
| `--debounce=<dur>` | Debounce interval for watch mode (default 200ms) |
| `--parallel=<n>` | Total concurrent test method budget (default: 2×GOMAXPROCS) |
| `--compile-parallel=<n>` | Concurrent compilation processes (default: NumCPU, auto-halved for -race/-msan/-asan) |
| `--input=<path>` | Replay a saved `go test -json` stream in `spec`/`summary` (`-` reads stdin) |
| `--github` | Emit GitHub annotations and step summary (auto-enabled in GitHub Actions) |
| `--coverage=<path>` | Coverage profile path for `summary` subcommand |
| `--bench` | Benchmark mode for `watch`: re-run benchmarks on change with ns/op deltas |
| `--save=<path>` | Save a benchmark run as a JSON baseline (`bench`) |
| `--against=<path>` | Compare a benchmark run against a saved baseline and print the delta table (`bench`; defaults to `bench.baseline`) |
| `--gate=<pct>` | Fail (exit 1) if the worst significant benchmark regression exceeds the threshold (`bench`) |
| `--for=<dur>` | Total fuzz time budget, split evenly across targets (`fuzz`; per-target share floors at 10s) |
| `--jobs=<n>` | Max concurrent fuzz targets (`fuzz`; default: max(1, GOMAXPROCS/2)) |
| `--no-harvest` | Disable table-test seed harvesting for this run |
| `--fuzz` | Generate fuzz round-trip skeletons (`scaffold`) |

### Disambiguation

The first argument is checked (literally) against the known subcommand set.
If it matches, it's consumed.
Otherwise, it's a package pattern.
`gotest ./watch` tests the `watch` package; `gotest watch` enters watch mode.

### Examples

```bash
gotest ./... -v -race                    # run tests (default mode)
gotest watch ./... -v                    # watch mode with verbose output
gotest clean ./...                       # remove orphaned generated files
gotest generate ./...                    # generate only, no test execution
gotest scaffold ./pkg/user.UserService   # generate suite skeleton
gotest spec ./...                        # run tests, show behavioral spec
gotest spec ./... --format=md --output=docs/spec.md
gotest ./... --min=80                    # fail if coverage below 80%
gotest ./... --ci -v -race               # CI mode (fail on F_ prefixes)
gotest ./... --debug                     # keep generated files for inspection
```

Registered `go test` flags pass through to the underlying run; unknown single-dash flags are rejected rather than forwarded (guard against typos) — use a bare `--` or `-args` to forward unvalidated flags verbatim.
`-json` is intercepted and re-emitted by the runner, and `-ldflags=-checklinkname=0` is injected into every `go test` invocation (required for the assertion tracer on Go 1.23+).
Each subcommand accepts its own subset of the `--gotest` flags; out-of-scope flags error with "not valid for this subcommand".
`--github` is `summary`-only; `--no-color` applies to `spec`/`summary`.

---

## Project Configuration (`.gotest.yml`)

Tool defaults can live in a `.gotest.yml`, found by walking up from the working directory to the first match or a `go.mod` boundary.
Precedence: CLI flag > `.gotest.yml` > built-in default.
Omitted keys fall back to the defaults; duration keys are pointers, so an explicit `0` is distinguishable from omission and disables the deadline.

```yaml
tags: "integration,e2e"    # -tags
timeout: 15m               # --timeout   (0 disables)
setup-timeout: 5m          # --setup-timeout (0 disables)
min-coverage: 80           # --min
parallel: 12               # --parallel
compile-parallel: 4        # --compile-parallel
debounce: 500ms            # --debounce (watch)
lint:
  skip: [stdlib-test, testify]   # lint rule IDs to disable project-wide
```

The file configures tool behavior only — never test semantics (see Design Principle 2).
Note: `--min=0` cannot override a yml `min-coverage` (0 is the flag's "unset" sentinel); remove the yml key to disable the gate.

---

## Test Suites

### Definition

A test suite is a Go struct whose name ends in `TestSuite`:

```go
type MyTestSuite struct {
    sut *MyService
}
```

Requirements:
- Name matches `^(?!ƒƒ_GOTEST_|_)(?:X_|F_)?.+TestSuite$`
- Methods use pointer receivers (`*MyTestSuite`, not `MyTestSuite`)
- Each suite must be its own `type` declaration (not block-style)
- Type names must be exported when the suite has test cases — otherwise a generation error (`go test` would never run its generated `Test` function).
  Unexported case-less `*TestSuite` types (embed bases, helpers) are allowed

### Lifecycle Hooks

All hooks are optional.
Unimplemented hooks become no-ops in the generated code.

| Method | Signature | Semantics |
|--------|-----------|-----------|
| `BeforeAll` | `func (s *Suite) BeforeAll(t *gotest.T)` | Once before the first test case |
| `AfterAll` | `func (s *Suite) AfterAll(t *gotest.T)` | Once after the last test case (via `t.Cleanup`) |
| `BeforeEach` | `func (s *Suite) BeforeEach(t *gotest.T)` | Before each test case (void form) |
| `BeforeEach` | `func (s *Suite) BeforeEach(t *gotest.T) *Ctx` | Before each test case (returning form — typed per-test context) |
| `AfterEach` | `func (s *Suite) AfterEach(t *gotest.T)` | After each test case (void form, via `defer`) |
| `AfterEach` | `func (s *Suite) AfterEach(t *gotest.T, ctx *Ctx)` | After each test case (context-aware form, via `defer`) |

**Void BeforeEach** (legacy form):

```
BeforeAll
├── BeforeEach → Test A → AfterEach (deferred)
├── BeforeEach → Test B → AfterEach (deferred)
AfterAll (via t.Cleanup — LIFO, runs after all subtests)
```

**Returning BeforeEach** (per-test context form):

```
BeforeAll
├── ctx := BeforeEach → Test A(ctx) → AfterEach(ctx) (deferred)
├── ctx := BeforeEach → Test B(ctx) → AfterEach(ctx) (deferred)
AfterAll (via t.Cleanup — LIFO, runs after all subtests)
```

The returning form creates a typed per-test context that flows through the lifecycle bracket.
Each test method receives its own context instance, enabling safe method-level parallelism without shared mutable state on the suite struct.

`AfterAll` is registered via `t.Cleanup` before `BeforeAll` runs, ensuring it executes even if `BeforeAll` registers its own cleanup functions (LIFO ordering).
`AfterEach` is `defer`-ed, ensuring it runs even when `t.Fatal()` triggers `runtime.Goexit()`.

Hooks accept either `*gotest.T` or `*testing.T`.

#### Context Consistency Rules

When `BeforeEach` returns a value, the following rules are enforced at generation time:

1. **Parallel requires per-test isolation** — `SuiteConfig{Parallel: true}` with a void `BeforeEach` is an error (its whole purpose is mutating shared suite state).
   A parallel suite with no `BeforeEach` is legal; its struct fields must then be write-once (set in `BeforeAll`, read-only in tests) — the generator cannot verify this, so it is the author's responsibility.
2. **All methods must accept context** — if `BeforeEach` returns a context, every test method must accept it as its second parameter
3. **AfterEach must accept context** — if `BeforeEach` returns a context and `AfterEach` exists, it must accept the context as its second parameter
4. **No orphan context** — `AfterEach` cannot accept a context parameter if `BeforeEach` does not return one
5. **Type consistency** — context parameter types must match `BeforeEach`'s return type across all methods
6. **Context type must be a pointer** — the context return/parameter type must be a pointer type

### Test Cases

Any exported method matching `^(?:X_|F_)?Test.+$` is a test case.
Each becomes a `t.Run` subtest in the generated code.

```go
func (s *Suite) TestSomething(t *gotest.T) {}
```

Test methods accept an optional typed context parameter as their second argument when the suite uses a returning `BeforeEach`:

```go
func (s *Suite) TestSomething(t *gotest.T, ctx *MyCtx) {}
```

#### Async Test Cases

Method names ending in `Async` with a trailing `done func()` parameter complete when `done()` is called, not when the method returns:

```go
func (s *Suite) TestDeliveryAsync(t *gotest.T, done func()) {
    s.bus.Subscribe(func(evt Event) {
        gotest.Equal(t, "created", evt.Kind)
        done()
    })
    s.bus.Publish(Event{Kind: "created"})
}
```

The generated code waits for `done()` or the test deadline, whichever comes first (with the deadline failing the test); calling `done()` more than once is safe.
With a returning `BeforeEach`, `done` comes last: `(t, ctx, done)`.
With `Timeout: 0` (no per-test deadline), a never-called `done()` waits until an outer bound fires — always give async suites a positive `Timeout`.
For polling-style asynchrony, prefer `Eventually`.

### Focus and Exclude

| Prefix | Effect |
|--------|--------|
| `F_` | **Focus** — only focused items run; all unfocused are skipped |
| `X_` | **Exclude** — always skipped, even if focused |
| *(none)* | Normal — runs unless something else is focused |

Rules:
1. If any suite has an `F_` prefix, all non-`F_` suites *in that test package* are skipped — focus scope is per package (and per ptest/pxtest variant), not repo-global
2. If any test case within a suite has an `F_` prefix, all non-`F_` cases in that suite are skipped
3. `X_`-prefixed items are always skipped, regardless of focus state
4. Exclude takes precedence over focus
5. Focus/exclude is evaluated independently for `ptest` and `pxtest` suites

Skipped suites produce a `t.Skipf("test suite was excluded by user")` stub.
Skipped test *cases* are simply omitted from the generated code — no per-case skip stub appears in the output.

The `--ci` flag performs a static analysis scan before generation and enables
snapshot read-only mode (missing baselines fail instead of being generated):

```
$ gotest --ci ./...
FAIL: focus prefix detected — remove F_ before merging:
  pkg/user/user_test.go:12    type F_UserServiceTestSuite
  pkg/payment/pay_test.go:28  PaymentTestSuite.F_TestCharge
```

CI mode is auto-detected from the standard `CI` environment variable when
`GOTEST_CI` is unset. Set `GOTEST_CI=0` to opt out.

### SuiteGuard

A suite can define a `SuiteGuard()` method that returns a reason string.
If non-empty, the entire suite is skipped at runtime with `t.Skipf("suite guard: %s", reason)`:

```go
func (s *MySuite) SuiteGuard() string {
    if os.Getenv("INTEGRATION_DB") == "" {
        return "INTEGRATION_DB not set"
    }
    return ""
}
```

Unlike `X_` (compile-time exclusion), `SuiteGuard` evaluates at runtime — useful for environment-dependent tests that should compile everywhere but only run when prerequisites are available.

### Parallel Execution

**Suite-level parallelism** is handled by the `gotest` CLI runner, which executes each suite's test binary as a separate subprocess.
This provides process-level isolation between suites.
The generated `func Test*` does **not** call `t.Parallel()` — parallelism is at the runner level, not the Go test scheduler level.

**Method-level parallelism** is opt-in via `SuiteConfig{Parallel: true}`.
Each generated subtest calls `it.Parallel()`; no explicit coordination is needed, because
`testing` itself only returns from the parent `t.Run` — and therefore only runs
`t.Cleanup` — after every parallel child has finished.
Method-level parallelism requires a returning `BeforeEach` — per-test state lives in the returned context, not on the shared suite struct.

```go
// Default: methods run sequentially within the suite
type MyTestSuite struct{}

func (s *MyTestSuite) TestAlpha(t *gotest.T) {}
func (s *MyTestSuite) TestBeta(t *gotest.T)  {}

// Opt-in: method-level parallel (requires returning BeforeEach)
type ParallelMethodTestSuite struct{}

type TestCtx struct{ conn *sql.Conn }

func (s *ParallelMethodTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.SuiteConfig{Parallel: true}
}
func (s *ParallelMethodTestSuite) BeforeEach(t *gotest.T) *TestCtx {
    return &TestCtx{conn: s.pool.Acquire()}
}
func (s *ParallelMethodTestSuite) AfterEach(t *gotest.T, ctx *TestCtx) {
    ctx.conn.Release()
}
func (s *ParallelMethodTestSuite) TestCreate(t *gotest.T, ctx *TestCtx) {}
func (s *ParallelMethodTestSuite) TestDelete(t *gotest.T, ctx *TestCtx) {}
```

When method-level parallelism is enabled, `AfterAll` still waits for every parallel
subtest to complete, but nothing in the generated code makes it wait: `t.Cleanup` — where
`AfterAll` runs — only fires after `TestMyTestSuite` returns, and `testing` does not
return from a parent test until all of its `t.Parallel()` children have finished. The
generated code must never wait on the parallel subtests itself (a `sync.WaitGroup`, a
channel): on panic, `testing` runs ancestor cleanups from the panicking goroutine while
the subtest that would release such a wait is still parked inside `t.Run`, so any
generated wait deadlocks against the panic unwind.
With `FailFast`, parallel suites additionally share a `ƒfailed` atomic flag: a failed subtest sets it, and subtests that start afterwards skip themselves.

On Windows, suite subprocesses run under job objects so that cancellation and teardown terminate the whole process tree.

### Generic Suites

Generic struct definitions are not code generation targets, but their instantiated type aliases are:

```go
type GenericTestSuite[T any] struct { value T }

func (s *GenericTestSuite[T]) TestSomething(t *gotest.T) {}

type StringTestSuite = GenericTestSuite[string]
type IntTestSuite    = GenericTestSuite[int]
```

Each alias produces an independent test suite.
Generic aliases only work in same-package tests (`ptest`), not `pxtest`.

### Fixtures

Structs ending in `Fixture` are package fixtures.
Structs ending in `SharedFixture` are cross-package shared fixtures.
Both use `(ctx context.Context) error` lifecycle signatures:

```go
// Package fixtures — run in test process
func (f *MyFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *MyFixture) AfterAll(ctx context.Context) error   { return nil }

// Shared fixtures — run in subprocess, shared across packages
func (f *MySharedFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *MySharedFixture) AfterAll(ctx context.Context) error   { return nil }
```

Package fixture `BeforeAll`/`AfterAll` receive `context.Background()` bounded by the fixture's configured timeout.
`BeforeEach` receives the test's `t.Context()`; `AfterEach` receives `context.Background()` (cleanup must proceed after test context cancellation).
Shared fixture hooks receive a context with the configured timeout (from `SharedFixtureConfig()` or defaults).

Package fixtures additionally support `BeforeEach`/`AfterEach`.
Shared fixtures do not — they run once in the subprocess, not per test case.

Test suites reference fixtures via named pointer fields:

```go
type BatchTestSuite struct {
    Fixture *E2ESetupFixture
}
```

Shared fixtures can be wired directly into standalone suites (without a package fixture) or into package fixtures:

```go
// Standalone — shared fixture wired directly into suite
type UserTestSuite struct {
    Postgres *PostgresSharedFixture
}

// Fixture-bound — shared fixture wired into package fixture, suite accesses it via fixture chain
type E2EFixture struct {
    Postgres *PostgresSharedFixture
}
type BatchTestSuite struct {
    Fixture *E2EFixture
}
```

Both paths produce the same lifecycle: deserialize from state file, call `Hydrate`, run tests, call `Dehydrate`.

Fixtures may be defined in a different package from the suite.
The resolver walks the type graph from targeted suites to discover all required fixtures, including cross-package dependencies.

Fixtures nest — a root fixture's hooks run first, wrapping the child's:

```
InfraFixture.BeforeAll
  APIFixture.BeforeAll
    Suite.BeforeAll
      InfraFixture.BeforeEach
        APIFixture.BeforeEach
          Suite.BeforeEach → Test → Suite.AfterEach
        APIFixture.AfterEach
      InfraFixture.AfterEach
    Suite.AfterAll
  APIFixture.AfterAll
InfraFixture.AfterAll
```

#### Fixture Runtime

Generated fixture code imports `pkg/gotestruntime`, the DAG runtime: `SetupFixtureDAG` (topological setup, parallel independent nodes, panic-to-error recovery), `Teardown` (reverse order), `FixtureOnce` (lazy setup on the first fixture-bound test), and `CountMatchingTests` (a `-run`-aware pending counter so teardown fires after the last matching test).
Its exported node types (`FixtureNode`, `SharedStateNode`, `MainConfig`, `FixtureDAG`) are the struct literals generated code populates.
Users never import it; it appears only in generated files.

#### SharedFixture State Transfer

SharedFixtures run in a generated subprocess.
State crosses the process boundary via JSON serialization.
The `Hydrate` method determines which fields are local (reconstructed in the test process) versus transferable (serialized from the subprocess).

**Additional lifecycle hooks for shared fixtures:**

| Method | Runs in | Signature | Semantics |
|--------|---------|-----------|-----------|
| `Hydrate` | Test process | `(ctx context.Context) error` | Reconstruct local resources from transferred state |
| `Dehydrate` | Test process | `(ctx context.Context) error` | Clean up locally-created resources |

`Hydrate` and `Dehydrate` are optional.
If a SharedFixture has only JSON-serializable exported fields, all fields transfer automatically and no `Hydrate` is needed.

**Field classification:**

Fields assigned to the receiver in `Hydrate` — directly, or in receiver methods called from `Hydrate` (one level deep) — are **local**.
They are excluded from serialization and reconstructed in the test process.
All other exported fields are **transferable**.

```go
type PostgresSharedFixture struct {
    ConnStr string            // transferable — read in Hydrate, not assigned
    Pool    *pgxpool.Pool     // local — assigned in connect(), called from Hydrate
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error {
    container, err := postgres.Run(ctx, "postgres:16")
    if err != nil {
        return err
    }
    f.ConnStr = container.MustConnectionString(ctx)
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

**Convention:** In `Hydrate`, assign to local fields.
Read transferred fields but do not reassign them — use local variables for any transformation.

**Classification algorithm:**

1. Parse `Hydrate`'s function body AST
2. Walk all statements (including inside `if`/`for`/`switch` blocks) to find receiver field assignments: `f.FieldName = expr` or `f.FieldName, _ = expr`
3. For method calls on the receiver (`f.methodName(...)`), look up the method on the same type and walk its body (same as step 2, without recursing further)
4. Fields found in steps 2–3 are **local**. All other exported fields are **transferable**

**Transfer lifecycle:**

```
Subprocess (compiled binary):
  sf.BeforeAll(ctx)              → provisions infrastructure, populates all fields
  serialize transferable fields  → JSON (local fields excluded)
  write to stdout, wait for SIGTERM
  sf.AfterAll(ctx)               → tears down infrastructure

CLI (gotest):
  read JSON from subprocess stdout
  write state files in the work dir — per-suite shared/<Suite>.json in the
  default streaming run; one global shared/state.json in batch modes
  set GOTEST_SHARED_STATE_FILE env var for test process

Test process:
  read shared/state.json         → deserialize into struct (transferable fields populated, local fields zero-valued)
  sf.Hydrate(ctx)                → reconstructs local resources from transferred state
  ... run test suites ...
  sf.Dehydrate(ctx)              → cleans up local resources
```

**DAG dependencies within the subprocess:**

When SharedFixture B depends on SharedFixture A, both run in the same subprocess.
After A.BeforeAll() completes, B receives an in-memory pointer to A — not serialized state.
B.BeforeAll() can access all fields that A.BeforeAll() set, including local fields like connection pools.
The serialization boundary only exists between the subprocess and test processes.

`BeforeAll` always sets transfer fields. Local fields only need to be set in `BeforeAll` when a dependent shared fixture accesses them — otherwise they may add an idle resource to the subprocess.
`Hydrate` is not called within the subprocess — it runs only in test processes after JSON deserialization.

**Validation at generation time:**

- Shared fixture types must not live in `internal/` packages.
  The setup subprocess compiles outside the module tree and cannot import `internal/` paths.
  Shared fixtures may freely depend on `internal/` packages — only the fixture type's own package path is restricted.
- Transferable field types are validated for JSON serializability: channels, funcs, and maps with non-string/non-integer keys are rejected recursively, with a suggestion to handle the field in `Hydrate` instead.
  Opaque types with zero exported fields serialize as `{}` (see Known Limitations)
- If `Hydrate` exists without `Dehydrate`, the generator emits an error
- `Hydrate`/`Dehydrate` signatures must be `(ctx context.Context) error` with pointer receiver

### Configuration

Every fixture and suite runs with sensible defaults.
Defining the optional marker method takes full ownership: the returned config is used as-is; without the marker, the defaults apply.

#### Config Types

```go
type FixtureConfig struct {
    Timeout    time.Duration // applied to BeforeAll/AfterAll via context.WithTimeout
    Retries    int           // additional BeforeAll attempts on failure
    RetryDelay time.Duration // pause between retry attempts
}

type SuiteConfig struct {
    Timeout      time.Duration // per-test-case deadline via t.Context()
    SetupTimeout time.Duration // BeforeAll/AfterAll deadline
    FailFast     bool          // stop suite on first failure
    Parallel     bool          // method-level parallelism (requires returning BeforeEach)
}
```

(Test-case retries are deliberately not offered — retrying flaky tests hides real defects; fixture `Retries` exist because infrastructure setup is legitimately flaky.)

#### Value Semantics

One rule for every timeout in the system:

| Value | Meaning |
|-------|---------|
| `> 0` | Use this duration |
| `0` (including an omitted field) | No timeout — the zero value opts out |
| `< 0` | Also no timeout (accepted side-effect; all gates check `> 0`) |

Defaults come from *absence*, uniformly at every layer: no marker method → `DefaultSuiteConfig()`/`DefaultFixtureConfig()`; no CLI flag → the flag's default; no `.gotest.yml` key → the CLI default.
This matches `go test -timeout 0` semantics.

#### Marker Methods

The code generator recognizes these exact signatures:

```go
func (f *MyFixture)       FixtureConfig()       gotest.FixtureConfig
func (f *MySharedFixture) SharedFixtureConfig()  gotest.FixtureConfig
func (s *MySuite)         SuiteConfig()          gotest.SuiteConfig
```

All three return the same `FixtureConfig` type for fixtures (package and shared) and `SuiteConfig` for suites.
The method name follows the type suffix convention.

Requirements (same conventions as lifecycle hooks):
- Pointer receiver matching the fixture/suite type name
- No parameters
- Single return value of the exact config struct type
- Invalid signatures produce a collector error

`SuiteConfig()` bodies are parsed statically (the generator needs `Parallel` at generation time), so they are restricted to: a literal return, a gotest preset call (`DefaultSuiteConfig`/`IntegrationSuiteConfig` — custom helpers would silently drop `Parallel`), or the compose form `cfg := <literal|preset>` followed by plain `cfg.<Field> = <value>` assignments (`Parallel` must be assigned a boolean literal).
`FixtureConfig()`/`SharedFixtureConfig()` bodies are ordinary Go — they run at test time.

#### Presets

| Preset | Timeout | Retries | Use case |
|--------|---------|---------|----------|
| `DefaultFixtureConfig()` | 2 min | 0 | Standard fixtures |
| `ContainerFixtureConfig()` | 5 min | 1 (5s delay) | Testcontainers, image pulls |
| `DefaultSuiteConfig()` | 30s (+ 30s setup) | — | Unit/integration tests |
| `IntegrationSuiteConfig()` | 2 min (+ 5 min setup) | — | Heavier integration tests |

#### Composing with Presets

Defaults and overrides combine through plain Go — start from a preset, override fields, return the result:

```go
func (s *OrderTestSuite) SuiteConfig() gotest.SuiteConfig {
    cfg := gotest.DefaultSuiteConfig() // 30s / 30s
    cfg.Parallel = true
    return cfg
}

func (f *InfraFixture) FixtureConfig() gotest.FixtureConfig {
    cfg := gotest.ContainerFixtureConfig()
    if os.Getenv("CI") != "" {
        cfg.Timeout = 10 * time.Minute
        cfg.Retries = 2
    }
    return cfg
}
```

A partial literal opts out of whatever it omits: `SuiteConfig{Parallel: true}` runs with no per-test deadline (the global `--timeout` still bounds the run).

#### Generated Behavior

**Package fixtures:** The test harness uses the marker's config when present (otherwise `DefaultFixtureConfig()`) to wrap `BeforeAll` in a retry loop with `context.WithTimeout` and wrap `AfterAll` cleanup with timeout.
Retry attempts are logged with attempt number.
As with suites, bounding the context is not being held to it: a fixture that declared its own config is additionally held to its `Timeout` *by verdict*, on both `BeforeAll` (`gotestruntime.RunFixtureSetup`) and `AfterAll` (`gotestruntime.RunFixtureTeardown`) — a lifecycle call that ignores its context and outruns the declared budget fails the fixture rather than passing.
A fixture with no marker gets the default bounds and no verdict; it is never failed against a number its author did not write.
A setup overrun on a *successful* return is terminal (`gotestruntime.ErrSetupOverran`): it is not retried — the work completed and the overrun is deterministic — and the fixture still gets its `AfterAll`, because the resources it built exist.

**Shared fixtures:** The generated subprocess uses `SharedFixtureConfig()` when present (otherwise `DefaultFixtureConfig()`), wrapping each SharedFixture's `BeforeAll(ctx)` in the same retry loop with `context.WithTimeout`, under the same declared-budget verdicts as package fixtures.
After `BeforeAll`, transferable fields (determined by Hydrate-assignment analysis) are serialized to stdout as JSON.
`AfterAll(ctx)` runs through `gotestruntime.RunFixtureTeardown` in the teardown handler: context-bounded always, and held to a declared `Timeout` by verdict, with a failed teardown reaching the runner through the process exit status.
The subprocess is shutdown-capable from birth and never tears down on its own initiative: it reports setup outcome (and its teardown budget) on the `_done` line, then waits for the runner's signal — only the runner knows when every suite has stopped using the fixtures — and a clean exit is the runner's sole proof that teardown ran and passed.
In the test harness, the deserialized fixture is hydrated via `Hydrate(ctx)` if present, and `Dehydrate(ctx)` is deferred for cleanup.

**Suites:** The test harness uses the marker's config when present (otherwise `DefaultSuiteConfig()`) to bound each phase's context — `gotestruntime.SetupT`/`TestT` apply `NewTWithDeadline` when the timeout is positive — and breaks the test case loop on first failure when `FailFast` is set.
Bounding the context is not the same as being held to it: `gotestruntime.RunSetup`, `RunTest` and `RunTeardown` additionally take a *budget* duration and fail the phase by verdict if it is still running once the budget elapses, but that budget is the zero value — nothing enforced — unless the suite declared a `SuiteConfig()` of its own. A suite with no marker gets bounded but unenforced defaults; a suite with a marker gets its own values as both the bound and the budget, verbatim.

**`NewTWithDeadline`:** Creates a `*gotest.T` with a context deadline.
`t.Context()` returns the deadline-aware context.

**`NewTWithContext`:** Creates a `*gotest.T` whose `t.Context()` is the context supplied by the caller, for the cases a deadline off `t.Context()` cannot express — injected values, or a lifetime that must outlive the test's own.
The caller owns the context; nothing in `gotest` cancels it.
This is what lets `AfterAll` run under a context that survives the cancellation the testing package performs immediately before cleanups.

#### Feature Interactions

- **Parallel suites:** in sequential suites, `FailFast` is checked between subtests.
  In method-parallel suites, a failed subtest sets a shared failure flag; parallel subtests that start afterwards skip themselves (best-effort — already-running subtests complete).
  Suite-level parallelism does not affect `FailFast` (each suite's subtests are independent).
- **Focus/Exclude:** Config applies after focus/exclude filtering.
  Skipped suites get unchanged skip stubs.
- **Global timeouts:** fixture/suite timeouts and the outer bounds are independent — fixture contexts derive from `context.Background()`, not from any parent deadline.
  `go test -timeout` acts by panicking the test binary; gotest's own `--timeout` cancels the whole pipeline at the process level.
  Whichever bound expires first ends the run.
- **Nested fixtures:** Each level resolves config independently — no inheritance between fixture levels.
- **Hydrate/Dehydrate:** state is deserialized and hydrated lazily — when the first fixture-bound test triggers setup (see fixtures.md, Execution Model).
  `Dehydrate` runs when the fixture-bound pending counter reaches zero, i.e. after the last matching fixture-bound test.
  `Hydrate` receives a context with the SharedFixture's configured timeout; `Dehydrate` receives `context.Background()`.

---

## The gotest.T Type

`*gotest.T` wraps `*testing.T` with a deliberately small surface:

| Method | Purpose |
|--------|---------|
| `T() *testing.T` | Escape hatch to the underlying `*testing.T` |
| `Context() context.Context` | Test-scoped context (deadline-aware under a suite timeout) |
| `Errorf`, `FailNow`, `Skipf` | Failure/skip primitives (assertions call these) |
| `Setenv(key, value)` | Delegates to `testing.T.Setenv` |
| `TempDir() string` | Delegates to `testing.T.TempDir` |
| `When(desc, fn)`, `It(desc, fn)` | BDD vocabulary (see below) |

(`gotestruntime.TestCase` — `func(*gotest.T)` — is the exported function type the generated exec trampoline accepts.)

Deliberately absent — the suite lifecycle replaces them:
`Log` (use assertions' message args), `Fatal`/`Fatalf` (use `FailNow` via assertions), `Cleanup` (use `AfterEach`/`AfterAll`), `Run` (use `When`/`It`), `Parallel` (use `SuiteConfig.Parallel`), `Helper` (the call-site tracer makes it unnecessary and harmful).
The `t-escape` lint rule flags escapes to these via `t.T()`.

---

## Goroutines Started by Tests

A panic on a goroutine a test starts directly is unrecoverable: Go terminates the whole
process without running any other goroutine's deferred work, so no `AfterEach`, no
`AfterAll` and no fixture teardown happens, and the panic is attributed to nothing in
particular. No framework can guard a goroutine it did not create.

`gotest.Go(t *gotest.T, fn func()) (wait func())` creates the goroutine for the caller
instead. It captures a panic in `fn` with the stack from where it happened and re-raises
it on the test's own goroutine, where `testing` reports it like any other test panic and
every cleanup still runs.

<!-- fence:pseudo -->
```go
// Work that finishes on its own: wait where the panic should surface.
wait := gotest.Go(t, func() { report = build(input) })
defer wait()

// Work that runs until something stops it — a server, a poller: do not wait
// inside the test. gotest.Go also registers the wait as t.Cleanup, which runs
// after AfterEach, so whatever stops the goroutine has already run by the time
// anything waits for it.
gotest.Go(t, func() { srv.Serve(l) }) // AfterEach closes the listener
```

A `defer wait()` in the second shape deadlocks instead: the test's own defers run before
`AfterEach`, so it would wait for a goroutine nothing has stopped yet. Calling the
returned `wait` more than once is safe.

The cleanup-registered `wait` has no timeout of its own: a goroutine that never
returns hangs the binary until `go test -timeout` fires, with no named verdict.

---

## Assertions

### Functional API

Type-safe generics with compile-time safety.
All functions accept any type implementing `testingT` (`Errorf()` + `FailNow()`) — works with `*gotest.T`, `*testing.T`, and `*gotest.R`.
Failure messages always include the user call site, resolved by an internal stack-frame tracer.
Never call `t.T().Helper()` — it degrades location reporting; the `t-escape` lint rule flags it.

<!-- fence:pseudo -->
```go
// Equality — [V any] catches cross-type comparison at compile time
gotest.Equal[V any](t, expected, actual V, msgAndArgs ...any)
gotest.NotEqual[V any](t, expected, actual V, msgAndArgs ...any)

// Zero / Empty
gotest.Zero[V comparable](t, value V, msgAndArgs ...any)
gotest.NotZero[V comparable](t, value V, msgAndArgs ...any)
gotest.Empty(t, object any, msgAndArgs ...any)
gotest.NotEmpty(t, object any, msgAndArgs ...any)

// Nil — for non-comparable nilables (slices, maps, funcs); runtime type guard
// rejects non-nilable types. Use Zero/NotZero for comparable nilables.
gotest.Nil(t, object any, msgAndArgs ...any)
gotest.NotNil(t, object any, msgAndArgs ...any)

// Bool
gotest.True(t, value bool, msgAndArgs ...any)
gotest.False(t, value bool, msgAndArgs ...any)

// Error
gotest.NoError(t, err error, msgAndArgs ...any)
gotest.Error(t, err error, msgAndArgs ...any)
gotest.ErrorIs(t, err, target error, msgAndArgs ...any)
gotest.ErrorAs[E error](t, err error, msgAndArgs ...any) E
gotest.ErrorContains(t, err error, contains string, msgAndArgs ...any)

// Collection
gotest.Contains(t, s, contains any, msgAndArgs ...any)
gotest.NotContains(t, s, contains any, msgAndArgs ...any)
gotest.Len(t, object any, length int, msgAndArgs ...any)
gotest.ElementsMatch[V comparable](t, listA, listB []V, msgAndArgs ...any)
gotest.Subset[V comparable](t, list, subset []V, msgAndArgs ...any)

// Comparison — [V cmp.Ordered] prevents comparing incomparable types
gotest.Greater[V cmp.Ordered](t, a, b V, msgAndArgs ...any)
gotest.GreaterOrEqual[V cmp.Ordered](t, a, b V, msgAndArgs ...any)
gotest.Less[V cmp.Ordered](t, a, b V, msgAndArgs ...any)
gotest.LessOrEqual[V cmp.Ordered](t, a, b V, msgAndArgs ...any)

// String / Regex
gotest.Regexp[P regexpPattern](t, rx P, str string, msgAndArgs ...any)

// Numeric
gotest.InDelta[V numeric](t, expected, actual V, delta float64, msgAndArgs ...any)

// Serialization — accepts string, []byte, json.RawMessage, io.Reader, or marshalable
gotest.JSONEq(t, expected, actual any, msgAndArgs ...any)

// Time
gotest.TimeWithin(t, expected, actual time.Time, tolerance time.Duration, msgAndArgs ...any)
gotest.TimeIsNow(t, ts time.Time, tolerance time.Duration, msgAndArgs ...any)

// Panic
gotest.Panics(t, f func(), msgAndArgs ...any) any

// Snapshot — auto-named from test path, or custom name
gotest.MatchSnapshot(t, value any, name ...string)

// Polling — see Async Assertions section
gotest.Eventually(t, waitFor, tick time.Duration, fn func(poll *gotest.R))
gotest.Consistently(t, waitFor, tick time.Duration, fn func(poll *gotest.R))

// Explicit failure
gotest.Fail(t, msgAndArgs ...any)

// Unwrap — panics on failure (no t parameter, enables multi-return expansion)
gotest.Must[V any](val V, ok any) V
```

Equality failures show both values in Go literal syntax:

```
Equal failed:
  expected: order{ID:"A-1", Amount:100, Items:[]string{"x", "y"}}
  actual:   order{ID:"A-1", Amount:150, Items:[]string{"x"}}
```

Large composite values (structs, maps, slices whose single-line form exceeds ~60 chars, up to 64 entries) are expanded to one field/entry per line — with sorted map keys — and a line-based `diff:` section follows:

```
Equal failed:
  expected: order{
  ID: "A-0001",
  Amount: 100,
  ...
}
  diff:
    - Amount: 100,
    + Amount: 150,
```

When only unexported fields differ (expansion cannot read them), the output falls back to `%#v`, which can.
The same diff renderer serves snapshot mismatches.

Runtime argument guards — each fails with a type error instead of guessing:

- `Contains`/`NotContains` accept a string (substring), slice/array (element, `DeepEqual`), or map (**key** presence)
- `Len` accepts slice, array, map, chan, string
- `Empty`/`NotEmpty` accept slice, map, array, chan, string, or pointer (recursively); other types are rejected — use `Nil`/`Zero`
- `ErrorIs` with a nil target and `ErrorContains` with an empty string are rejected — use `NoError`/`Error`
- `Must`'s second argument may be `nil`, `bool`, or `error`; anything else panics

The assertion engine has zero external dependencies — `reflect.DeepEqual`, `cmp.Compare`, `fmt.Sprintf("%#v")`, and a minimal inline diff renderer.

---

## BDD Vocabulary

### t.When() / t.It()

`When` groups context.
`It` specifies behavior.
Both map to `t.Run` under the hood — the distinction is purely semantic.

```go
func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.When("email is valid", func(w *gotest.T) {
        w.It("creates the user", func(it *gotest.T) {
            err := s.svc.Create(ctx, validUser)
            gotest.NoError(it, err)
        })
    })

    t.When("email already exists", func(w *gotest.T) {
        w.It("returns ErrDuplicate", func(it *gotest.T) {
            s.svc.Create(ctx, validUser)
            err := s.svc.Create(ctx, validUser)
            gotest.ErrorIs(it, err, ErrDuplicate)
        })
    })
}
```

### gotest.Each()

Table-driven tests with automatic subtest naming.

Iterator API (Go 1.23+):

```go
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
```

Uses `Desc` or `Name` field for the subtest name, falls back to `#0`, `#1`, etc.
A `FailNow` inside an entry aborts the remaining entries (the loop propagates it to the parent); `break` is supported and stops cleanly.

---

## Async Assertions

### *gotest.R — Assertion Recorder

`*R` is an assertion recorder that captures assertion outcomes without propagating them to the test runner.
It satisfies the same `testingT` contract as `*testing.T` (`Errorf` + `FailNow`), making it the callback type for `Eventually` and `Consistently`.
All assertion functions work with `*R` just as they do with `*T` or `*testing.T`.

<!-- fence:pseudo -->
```go
type R struct { ... }
func (r *R) Errorf(format string, args ...any)
func (r *R) FailNow()
func (r *R) Failed() bool
func (r *R) Message() string

func Record(fn func(*R)) *R
```

`Record` runs `fn` with a fresh `*R` in a dedicated goroutine (required because `FailNow` calls `runtime.Goexit`).

### Polling

```go
gotest.Eventually(t, 5*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
    result, err := s.store.Get("key")
    gotest.NoError(poll, err)
    gotest.Equal(poll, "completed", result.Status)
})

gotest.Consistently(t, 2*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
    gotest.Equal(poll, 0, s.counter.Value())
})
```

The `poll *gotest.R` captures assertion failures without propagating them to the test runner.
On timeout, `Eventually` reports the last poll's assertion failures.
`Consistently` fails on first assertion failure and reports which poll failed.

---

## Snapshot Testing

```go
gotest.MatchSnapshot(t, result)               // auto-named from test
gotest.MatchSnapshot(t, result, "variant")    // custom snapshot name
```

- One `.snap` file per **top-level** test at `testdata/__snapshots__/<TestName>.snap` (suffix `_ext` for external `_test` packages); subtests become `=== SNAP <subtest path> ===` sections within it, with the custom name appended to the key
- Accepted value types: `string` (including named string types), `[]byte`, `encoding.TextMarshaler`, `fmt.Stringer`, `json.Marshaler`, `error`, `io.Reader` — arbitrary structs and nil values are rejected with a type error
- First run: create snapshot, pass; subsequent runs: compare, fail with diff and the hint `Run with GOTEST_UPDATE_SNAPSHOTS=1 to update`
- Update: `gotest --update-snapshots ./...` (sets `GOTEST_UPDATE_SNAPSHOTS=1`; use the env var directly under plain `go test`)
- CI mode (`--ci` / `GOTEST_CI=1`): read-only — a missing baseline fails instead of being created
- Safe under parallel tests: snapshot files are guarded by per-file mutexes

---

## Scaffolding

```
$ gotest scaffold ./pkg/user.UserService
```

Generates a test suite skeleton with `BeforeEach`, per-method `Test*` stubs, and method signatures as comments.
Methods returning `error` get happy-path and error-case stubs.

Interface targets are detected automatically and generate a generic contract suite:

```
$ gotest scaffold io.ReadCloser
```

File targets scaffold a suite over the file's exported package-level functions:

```
$ gotest scaffold ./pkg/user/service.go
```

Output is written as `<snake_case>_suite_test.go` next to the target; existing files are never overwritten.
The subcommand takes no flags — unknown flags are rejected.
Uses `packages.Load` for type introspection.

---

## Migration

```
$ gotest migrate ./...
```

Converts testify/suite tests:
1. Renames suite struct to `*TestSuite` convention
2. Renames lifecycle hooks (`SetupSuite` → `BeforeAll`, `TearDownSuite` → `AfterAll`, etc.)
3. Transforms assertion calls (`s.Require().Equal(a, b)` → `gotest.Equal(t, a, b)`)
4. Removes `suite.Run` boilerplate, the `suite.Suite` embedding, and testify imports (dropping `testing` when it becomes unused)
5. Injects the `t *gotest.T` parameter into lifecycle and test methods; rewrites standalone `s.T()` to `t.T()`
6. Transforms both `s.Require().X(...)`/`s.Assert().X(...)` chains and `assert.X(s.T(), ...)`/`require.X(...)` forms

Not converted: direct embedded-suite calls (`s.Equal(...)`) — the removed `suite.Suite` embedding makes them compile errors, and they are annotated with a rewrite hint.

Anything the tool cannot convert is annotated in place, so nothing is silently skipped:

- `// TODO(gotest-migrate): unsupported testify hook <Name> — convert manually` above unconverted lifecycle hooks (`SetupSubTest`, `TearDownSubTest`, `BeforeTest`, `AfterTest`, `HandleStats`)
- `// TODO(gotest-migrate): unmapped assertion <Name> — convert manually` above assertion calls outside the mapping table (e.g. `ErrorAs`, `Eventually`, `InDelta`)
- `// TODO(gotest-migrate): unconverted assertion <Name> — embedded-suite call; rewrite as gotest.<Name>(t, ...)` above direct embedded-suite calls with mapped names

---

## Behavior Specification

```
$ gotest spec ./pkg/user

UserService
  Create
    when email is valid
      ✓ creates the user (8ms)
      ✓ sends a welcome email (120ms)
    when email already exists
      ✓ returns ErrDuplicate (<1ms)
  Delete
    ✓ soft-deletes the user (5ms)
    ~ hard-deletes after 30 days — SKIPPED (<1ms)

2 suites, 5 behaviors: 4 passed, 1 skipped
```

Internally runs `go test -json`, parses the event stream, reconstructs the suite→method→When/It hierarchy from `/`-separated test paths, and strips Go naming conventions for display.

Output formats:

```bash
gotest spec ./...                                    # terminal (color tree)
gotest spec ./... --no-color                         # terminal (plain)
gotest spec ./... --format=md --output=behavior.md   # markdown
gotest spec ./... --format=json                      # machine-readable tree
gotest spec --input=events.json                      # replay a saved go test -json stream (no test run)
gotest ./... -v --spec                               # spec view instead of default output
```

---

## Failure Summary

```
$ gotest summary ./...
```

The CI-facing counterpart to `spec`: passing runs collapse to a one-line summary; failing runs show only the failed behaviors plus package-level diagnostics.
Runs the same pipeline as `spec` over `go test -json` events.

Flags:

```bash
gotest summary ./... --coverage=coverage.out   # append a per-package coverage table
gotest summary ./... --min=80                  # fail below the coverage threshold
gotest summary ./... --github                  # ::error annotations + $GITHUB_STEP_SUMMARY
go test -json ./... | gotest summary --input=- # post-process an existing JSON stream
```

`--github` is auto-enabled when `GITHUB_ACTIONS=true`.
The coverage table is statement-weighted with block deduplication (see Coverage Model).
Formats: terminal (default), `md`, `json`.
The repository's composite GitHub Action (`action.yml`) wraps this subcommand.

---

## Machine-Readable Discovery

```
$ gotest discover ./...
```

Emits the static suite model as JSON — the integration surface for editors and AI tooling (the VS Code extension's test explorer runs on it).
No tests are executed.

```
{ "packages": [ {
    "importPath": …, "dir": …, "modulePath": …, "testOnly": bool,
    "suites": [ {
      "name": …, "file": …, "line": …, "col": …,
      "parallel": bool, "focused": bool, "excluded": bool, "guarded": bool,
      "lifecycle": ["BeforeAll", …],
      "fixtures": ["E2ESetupFixture", …],
      "methods": [ { "name": …, "file": …, "line": …, "col": …,
                     "focused": bool, "excluded": bool, "parallel": bool } ]
    } ]
  } ],
  "warnings": [ { "importPath": …, "file": …, "line": …, "col": …, "message": … } ] }
```

File paths are basenames; positions are 1-based.
`methods[].parallel` is reserved and always `false` (parallelism is a suite-level property).
Respects `-tags`.

---

## Debugging Shared Fixtures (`prepare`)

```
$ gotest prepare ./tests/e2e
```

Starts the shared-fixture subprocess, then prints a machine-readable JSON line and blocks until SIGTERM/SIGINT (teardown runs on signal):

```
{"overlayFile": "<path to overlay.json>", "dir": "<workdir>", "stateFile": "<shared state file>"}
```

IDE debuggers (e.g. Delve via the VS Code extension) use this contract to run individual suites against live shared fixtures.

---

## Watch Mode

```bash
gotest watch ./... -v
gotest watch ./... --spec
```

1. Initial run: full generate → test → cleanup cycle
2. Watch filesystem for `.go` file changes (via `fsnotify`); dot-directories, `vendor/`, `testdata/`, and `node_modules/` are excluded
3. On change: re-run the *changed directories* only (no reverse-dependency analysis — packages importing the changed one are not re-run)
4. Debounce on rapid changes (default 200ms; `--debounce` flag or `.gotest.yml` `debounce`)
5. Terminal clear between runs

Focus integration: `F_`-prefixed suites during watch mode create a tight feedback loop — only focused tests re-run on each save.

Limitations: only Write/Create/Rename events trigger re-runs (file deletion does not), and directories created after watch startup are not added to the watcher.

---

## Code Generation

**Generate only** (no test execution):

```bash
gotest generate ./...
```

Writes generated files directly to package directories.
Use case: `//go:generate gotest generate ./...` for checked-in generated files.

**Clean up** orphaned files:

```bash
gotest clean ./...
```

Walks directories and removes files matching `gotest_p(x)?suite_test.go`.

### Generated Files

Per package directory, up to two files:

| File | Purpose |
|------|---------|
| `gotest_psuite_test.go` | Same-package (white-box) test suites |
| `gotest_pxsuite_test.go` | External-package (black-box) test suites |

The `gotest_` prefix prevents naming collisions with user code.
Files contain a `// Code generated` header.

### Generated Structure

For each suite, the generated code creates a wrapper struct with no-op fallbacks for unimplemented hooks, then a `func Test*` that wires lifecycle and inline `t.Run` blocks:

```go
type ƒƒ_GOTEST_MyTestSuite struct { MyTestSuite }

func TestMyTestSuite(t *testing.T) {
    s := &ƒƒ_GOTEST_MyTestSuite{}
    ƒcfg := gotest.DefaultSuiteConfig()
    ƒbudget := gotest.SuiteConfig{}

    t.Cleanup(func() {
        gotestruntime.RunTeardown(t, ƒcfg.SetupTimeout, ƒbudget.SetupTimeout, s.AfterAll)
    })
    gotestruntime.RunSetup(t, ƒcfg.SetupTimeout, ƒbudget.SetupTimeout, s.BeforeAll)

    t.Run("TestSomething", func(it *testing.T) {
        ttt := gotestruntime.TestT(it, ƒcfg.Timeout)
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        gotestruntime.RunTest(ttt, ƒbudget.Timeout, func() {
            ƒƒ_GOTEST_exec(s.TestSomething, ttt)
        })
    })
    if ƒcfg.FailFast && t.Failed() {
        return
    }
}
```

The sample shows a sequential suite without a config marker. `ƒcfg` always carries a
config — the declared one, or `DefaultSuiteConfig()` when there is no `SuiteConfig()`
method — and its `Timeout`/`SetupTimeout` bound the context that `gotestruntime.TestT`
and `RunSetup`/`RunTeardown` hand to each phase. `ƒbudget` is different: it is the
config `RunTest`, `RunSetup` and `RunTeardown` are told to *enforce by verdict*, and it
stays a zero-value `gotest.SuiteConfig{}` unless the suite declared one. With a
`SuiteConfig()` marker, `ƒcfg := s.MyTestSuite.SuiteConfig()` replaces the default and
`ƒbudget := ƒcfg` — the same values the author wrote now double as the enforced budget,
used verbatim, including a zero or negative duration meaning no deadline.
Ordering nuance: in the returning-`BeforeEach` form, `ctx := s.BeforeEach(ttt)` runs *before* `defer s.AfterEach(ttt, ctx)` is registered — a fatal failure inside `BeforeEach` means `AfterEach` never runs.
In the void form shown above, the deferred `AfterEach` is registered first and runs even when `BeforeEach` fails fatally.
Method-parallel suites additionally emit the `ƒfailed` coordination described under Parallel Execution.

---

## Linter

Available as the `gotest lint` subcommand, built on `go/analysis`. `pkg/lint` exports the analyzer for external `go/analysis` drivers; there is no golangci-lint plugin — the linter runs as its own step alongside a project's existing linter:

```bash
gotest lint ./...                                        # installed
go run github.com/mvrahden/go-test/cmd/gotest lint ./... # without installing
```

Rules (IDs are canonical — used by `//nolint:<rule>`; `.gotest.yml` `lint.skip` accepts any non-integrity rule):

Rules are grouped into three tiers by what breaks when a finding is ignored; the tier derives the suppression policy.

**Integrity** — test outcomes can lie or resources can leak. Suppressible per line only, never project-wide.

| Rule ID | Detects |
|---------|---------|
| `focus` | Committed `F_` prefixes on suites or methods |
| `receiver` | Suite methods with value receivers instead of pointer receivers |
| `lifecycle-typo` | Method names within Levenshtein distance ≤ 2 of a lifecycle hook (`BeforAll`, `AfterEeach`) |
| `lifecycle-pair` | `BeforeAll` without `AfterAll` — resources may leak |
| `x-lifecycle` | `X_` prefix on a lifecycle hook — a no-op |
| `test-signature` | Test methods not accepting `*gotest.T` (or `*testing.T`) |
| `suite-lifecycle` | `Cleanup`/`Parallel`/`Run` via `t.T()` — they bypass the suite lifecycle (split out of `t-escape`; committed `//nolint:t-escape` comments still suppress it) |
| `poll-scope` | Assertions inside `Eventually`/`Consistently` callbacks using the outer `t` instead of `poll` |
| `assertion-type-guard` | `Nil`/`Empty` on types their runtime guards would reject |
| `generated-file` | `gotest_p(x)suite_test.go` files present in source control |

**Expressiveness** — the test is correct but says it worse. Suppressible per line or project-wide via `lint.skip`.

| Rule ID | Detects |
|---------|---------|
| `assertion-simplify` | Simplifiable assertions: `True(t, a == b)` → `Equal`, `Len(t, x, 0)` → `Empty`, … |
| `assertion-redundant` | An assertion made redundant by the next one on the same argument |
| `fail-guard` | `if cond { gotest.Fail(…) }` guards (also halting `Fatal`/`Fatalf`/`FailNow` bodies) — the assertion expresses the check directly; `\|\|` conditions and `else if` chains decompose into sequential assertions, non-halting `Errorf` bodies and init-scoped guards report without a fix; fires only in files that import gotest |
| `t-escape` | Unnecessary `t.T()` convenience escapes: `Errorf`/`FailNow`/`Skipf`/`Setenv`/`TempDir` (available on `gotest.T`), `Skip`/`SkipNow` (use `Skipf`), `Helper` (degrades call-site reporting), `Log`/`Fatal`/`Fatalf` (use assertions and their message args) |

**Migration** — legitimate coexistence, nudged. Suppressible per line or project-wide via `lint.skip`.

| Rule ID | Detects |
|---------|---------|
| `stdlib-test` | `func TestX(*testing.T)` — migration aid suggesting a suite method (fires in any package; coexisting stdlib tests are legitimate, see The Two Runners) |
| `testify` | Any `github.com/stretchr/testify/*` import — migration incomplete |

One construct yields one finding: integrity rules own the constructs they flag, and expressiveness rules stand down on them (a guard whose body escapes the poll scope gets the `poll-scope` finding, not a `fail-guard` rewrite). The assertion surface itself is derived from the gotest package's type information, so the linter cannot drift from the API and lookalike names from other packages never match expressiveness rules. `poll-scope` is the deliberate exception: as an integrity rule it also matches assertion-shaped names from foreign libraries (an escaping testify assertion breaks the poll loop exactly as a gotest one would), and it recognizes polling contexts by the callback's typed `*gotest.R` parameter rather than the callee, so wrapped or re-exported polling helpers stay covered.

Suppression and configuration:

- `//nolint:<rule>` on the diagnostic's line, or in a standalone comment block ending on the line directly above it (a comment trailing other code does not reach the next line); on the `package` line it applies file-wide
- `.gotest.yml` → `lint.skip: [<rule>, ...]` disables non-integrity rules project-wide
- `.gotest.yml` `lint.skip` naming an integrity rule is a hard error — integrity rules can only be suppressed per line; unknown rule IDs are also a hard error
- Flags: `-fix` applies suggested fixes; `-skip-<rule>` for every non-integrity rule; `-disable-nolint`
- Exit codes: `0` no findings; `1` uncompilable target packages (the preflight fails loudly — nothing was proven about them); `2` usage or configuration error; `3` findings reported

GitHub annotations (subcommand only): `gotest lint --github` additionally emits one `::error file=…,line=…,col=…,title=<rule>::<message>` workflow command per finding on stdout and appends a findings table (rule, location, message) to `$GITHUB_STEP_SUMMARY` — the complete record when GitHub caps rendered annotations. Like `summary`, the mode is implied when `GITHUB_ACTIONS=true`, so an existing CI lint step gains PR annotations without workflow changes. Annotation paths are relative to the working directory (the repository root in a workflow). Plain-text findings, exit codes, `.gotest.yml` handling, and `//nolint` semantics are unchanged. Driver flags this mode does not own (`-fix`, `-json`, `-c`, …) defer to the `go/analysis` driver, which keeps their exact semantics but cannot emit annotations.

---

## CI Integration

The repository root ships a composite GitHub Action (`action.yml`, "Go - Test Suites") that resolves `gotest` from your `go.mod` by default (`version: gomod` runs it via `go run`; a tag installs a binary) and runs `gotest summary --github`:

```yaml
- uses: mvrahden/go-test@<version>
  with:
    packages: ./...
    race: true
    coverage: true
    min-coverage: "80"
```

Inputs: `packages`, `race`, `coverage`, `min-coverage`, `flags`, `go-test-flags`, `version`.
Outputs: `exit-code`, `coverage`.

Manual setup works without the action:

```yaml
- run: go install github.com/mvrahden/go-test/cmd/gotest@latest
- run: gotest --ci ./... -v -race -coverprofile=coverage.out
- run: gotest spec ./... --format=md --output=behavior-spec.md
```

Exit codes: 0 = pass, 1 = test failure, 2 = usage, generation, or build error (stricter than `go test`, which exits 1 on build errors).

Every package a pattern matches ends in exactly one verdict. A package that fails to load or compile — a syntax error, a type error, a nonexistent path — is a failed package (exit 2): its diagnostics are booked into the same output stream as suite results, grouped under a `# <import-path>` header, so text output, `--json` events, `spec`, `summary`, and `--input` replays all carry the failure. Packages that did build still run; one broken package never blocks the rest. `run`, `watch`, `spec`, and `summary` book-and-continue this way; `generate` and `prepare` fail fast instead, because generated output for an unbuildable package is meaningless; `discover` reports such packages with `"broken": true` and their diagnostics as warnings. "no test suites to run" (exit 0) is reserved for runs where every matched package loaded and none defined suites.

The `--ci` flag fails the run when any `F_` (focus) prefix is committed, preventing accidental focus leaks in CI.

---

## Environment Variables

User-facing:

| Variable | Effect |
|----------|--------|
| `GOTEST_UPDATE_SNAPSHOTS=1` | Regenerate snapshot baselines (what `--update-snapshots` sets; the only mechanism under plain `go test`) |
| `GOTEST_CI` | `1`/`true` forces CI mode; any value suppresses auto-detection from `CI`; unset → auto-detect |
| `GOTEST_CACHE_DIR` | Overlay write-cache location (default `os.UserCacheDir()/gotest`; entries evicted after 7 days) |

Internal protocol between the CLI and test/subprocess boundaries (may change without notice): `GOTEST_SHARED_STATE_FILE` (shared-fixture state file for the test process), `GOTEST_TEARDOWN_BUDGET_FILE` (teardown grace-period handshake).

Consumed (read-only): `CI` (CI-mode auto-detection), `GITHUB_ACTIONS` (auto-enables `--github`), `GITHUB_STEP_SUMMARY` (step-summary target for `summary --github`).

---

## Coverage Model

The Go coverage profile is the single source of truth at every level of aggregation.
No filesystem scanning, line counting heuristics, or mixed data sources.

Two components implement this model: the **VS Code extension** (`vscode-gotest/` — Test Coverage sidebar, Copy Coverage report, gutter annotations) implements all of it; the **CLI** implements the statement-weighted metric with block deduplication in `summary --coverage`, and delegates `--min` to `go tool cover -func`.
The primary/supplementary file-scope rule below is extension-only: the CLI consumes a single merged profile with no origin information, so it cannot distinguish supplementary entries.

### Primary Metric: Statement-Weighted Coverage

Each profile entry has the form:

```
file:startLine.startCol,endLine.endCol numStatements count
```

`numStatements` is the number of Go statements in a basic block as determined by the compiler's instrumentation.
Coverage at any scope (file, directory, workspace) is:

```
covered = sum of numStatements for all blocks in scope where count > 0
total   = sum of numStatements for all blocks in scope
percentage = covered / total (or 0% if total == 0)
```

A directory's percentage is the weighted sum of its children (weighted by statement count, not an average of percentages).
A parent's number is always derivable from its children.

This is the sole numeric metric displayed in both the Test Coverage sidebar bar and the Copy Coverage report.
The sidebar and report must always show identical `covered/total` values for the same scope.

Declaration (function) coverage is not a sidebar or report metric.
Function-level annotations are available in the editor gutter via `loadDetailedCoverage` for navigation purposes only.

### Block Deduplication

When `-coverpkg` is used, each file may appear in multiple test binary profiles.
Blocks are deduplicated by `file + startLine.startCol,endLine.endCol` identity: for each unique block, take `max(count)` across all entries.
This matches `go tool cover` behavior.

### Supplementary Coverage (Cross-Package Profiles)

Test-only packages (packages with no production `.go` files) run with `-coverpkg=./...`, which instruments all packages in the module.
This cross-package profile is **supplementary**: it can increase block counts for files that primary profiles already cover, but does not add new files to the aggregate.

- **Primary profiles** come from a package's own `go test` run.
  They define the file scope.
- **Supplementary profiles** come from test-only packages with `-coverpkg=./...`.
  After block deduplication, only files present in the primary scope are retained.
- If no primary profiles exist, supplementary profiles are treated as primary (fallback).

### Breadth Indicator

A supplementary signal showing how many source files have coverage data vs. how many exist:

- **Source files:** Non-test Go files (`*.go` excluding `*_test.go`) per directory, from the filesystem.
- **Profile files:** Files from the source file set with at least one entry in the primary coverage scope, regardless of whether that entry has `count > 0`.
  A file at 0% was instrumented and counts as profiled.

The percentage answers: *"How well-tested is the code my tests reach?"*
The file count answers: *"How much of my codebase do my tests reach at all?"*

### What NOT to Do

- Do not count lines of code, lines with tokens, or non-blank lines as a denominator.
  The profile's `numStatements` is the only valid statement metric.
- Do not include `_test.go` files in any denominator.
- Do not average per-file percentages to compute a directory percentage.
  Use weighted sums by statement count.
- Do not invent a filesystem-based metric as a fallback when the profile is sparse.
  A sparse profile is honest.
- Do not display declaration/function coverage as a separate metric in the sidebar or report.
- Do not let supplementary (cross-package) profiles expand the file scope beyond what primary profiles define.

---

## Advanced Patterns

### Contract Testing via Generic Suites

Generic type definitions + instantiated aliases = reusable behavioral specifications:

```go
type StorageTestSuite[T Storage] struct {
    factory func() T
    store   T
}

func (s *StorageTestSuite[T]) TestPutAndGet(t *gotest.T) { /* ... */ }

type MemoryStorageTestSuite = StorageTestSuite[*MemoryStorage]
type RedisStorageTestSuite = StorageTestSuite[*RedisStorage]
```

Each alias produces an independent conformance report.

### Resource Lifecycle Guarantees

1. `AfterAll` is registered via `t.Cleanup` BEFORE `BeforeAll` runs
2. `t.Cleanup` runs in LIFO order — user cleanups in `BeforeAll` run before `AfterAll`
3. `AfterEach` is `defer`-ed — runs even on `t.Fatal()` / `runtime.Goexit()`
4. In method-parallel suites (`Parallel: true`), every `t.Parallel()` subtest completes before `AfterAll` — `testing` does not return from the parent test, and therefore does not run `t.Cleanup`, until they have
5. `t.Fatal()` in `BeforeAll` skips the entire suite
6. `t.Skip()` in `BeforeAll` marks the suite as skipped
7. In context-aware suites (returning `BeforeEach`), each test receives its own context — `AfterEach` receives the same context for cleanup

---

## Non-Goals

### Test dependency ordering

Tests that depend on other tests are brittle.
Each test sets up its own preconditions via `BeforeEach`.

### Mocking framework

Mocking is orthogonal to test organization.
`gomock`, `mockery`, `moq`, and counterfeiter all work inside suites unchanged.

### Decorator / annotation syntax

Go doesn't have decorators.
Naming conventions are grepped, autocompleted, and understood at a glance.
Struct tags are hidden in backtick strings.

### Runtime suite registration

`suite.Run(t, new(MySuite))` is testify's approach.
The entire point of `go-test` is to generate that boilerplate.

### Cross-package suite inheritance

Breaks Go's package isolation model.
Cross-package sharing uses helper functions or fixtures, not suite inheritance.

### Replacing `go test` output

The `spec` subcommand and `--spec` flag are post-processing views over `go test -json` data.
They never alter how the tests themselves are executed — the spec view is rendered from the same `go test -json` event stream.

---

## Architecture

### Package Graph

```
cmd/gotest/                  CLI entrypoint, subcommands, arg handling
  ├── internal/lint/           go/analysis analyzer (lint subcommand)
  └── internal/gotestrunner/   Suite generation I/O, go test execution, overlay
        └── internal/gotestgen/   Package loading, collection, fixture resolution, rendering
              └── internal/gotestast/   AST analysis, spec model, regex classification

internal/config/             .gotest.yml project configuration loading
internal/gotestspec/         Spec tree builder and renderers (terminal, markdown, json)
internal/x/                  Small generic helper libraries (slices)
internal/scaffold/           Type-to-suite skeleton generator
internal/migrate/            testify/suite AST transformer
internal/refactor/           AST refactoring tools (focus/exclude toggle)

pkg/gotest/                  User-facing API (T, R, assertions, Each, Eventually, Consistently, MatchSnapshot)
  └── internal/assert/         Core assertion implementation (pure stdlib)
  └── internal/snapfile/       Snapshot file I/O and diffing

pkg/gotestruntime/           Fixture DAG runtime imported by generated fixture code
pkg/lint/                    Exported analyzer for external go/analysis drivers
internal/protocol/           CLI↔test-process env var and naming constants
internal/about/              Build metadata, file naming constants
```

### Code Generation Pipeline

| Stage | Input | Output |
|-------|-------|--------|
| **Load** | Package pattern (e.g., `./...`) | `[]*LoadResult` wrapping `*packages.Package` with syntax, types, module info |
| **Collect** | Package AST | `TestSuiteSpecSet` — suites with attached methods, local fixture specs |
| **Reduce** | `TestSuiteSpecSet` | Effective set after focus/exclude applied |
| **Resolve** | Effective suites + local fixtures | `ResolveResult` — fixture trees, shared fixture info, suite→fixture bindings |
| **Render** | Reduced spec + resolve result | Formatted Go source bytes |

Collection is a two-pass AST traversal: find type declarations matching suite patterns, then attach methods by matching receiver types.
Resolution is demand-driven: it starts from targeted suites and walks the Go type graph recursively (via `types.Named`, `types.Struct`, `types.MethodSet`) to discover all required fixtures, including cross-package dependencies.

### Key Invariant

The pipeline is always: **static analysis → code generation → standard `go test`**.
The only runtime components are the thin `gotest.T` wrapper (with its assertion engine) and, for fixture-bound suites, the `pkg/gotestruntime` DAG orchestrator.
If a feature can't be implemented as (a) generated code, (b) a method on `gotest.T`, (c) fixture orchestration in `pkg/gotestruntime`, or (d) post-processing of `go test -json`, it doesn't belong in this project.

---

## Known Limitations

1. **Generic aliases in `pxtest`:** Go does not allow defining methods on aliases of types from other packages.
   Generic suite aliases only work in same-package tests.

2. **`go.work` required for cross-module tests:** The generator golden tests require `go.work` (`go work init . && go work use ./examples`) and fail without it.

3. **No incremental generation:** The tool regenerates all suite files on every run.
   There is no staleness detection.
   (The overlay cache — `--no-cache`, `GOTEST_CACHE_DIR` — is a post-generation, content-addressed write cache: it dedupes disk writes, not generation work.)

4. **Hydrate method walking depth:** The generator follows receiver method calls from `Hydrate` one level deep to classify local fields.
   Assignments hidden behind two or more levels of indirection are not detected.
   Opaque types (zero exported fields) are unaffected — they serialize harmlessly as `{}` and `Hydrate` overwrites the value.
