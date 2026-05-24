# gotest Specification

Go tests that write themselves, organize themselves, and explain themselves.

`gotest` closes the gap between `func TestX(t *testing.T)` and a well-organized test suite through code generation.
Developers write structs, name them well, and the tool handles the rest.
No runtime dependencies.
No reflection.
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

No config files.
No struct tags.
No annotations.
No registration calls.
A developer reads the naming conventions once and never opens documentation again.

### 3. Zero runtime cost

The tool generates code at build time and cleans it up after.
At test execution time, there is no `go-test` code in the call stack (except the thin `gotest.T` wrapper).
No reflection, no interface dispatch, no type assertions.
The generated code is what a careful developer would write by hand.

### 4. Invisible until needed

A developer who has never heard of `go-test` can read a test suite struct and understand what it does.
A developer who runs `go test` directly (without the CLI) gets a compilation error for the missing generated file — not silent wrong behavior.

### 5. Adopt incrementally, eject freely

Existing `func Test*` tests coexist with suites in the same package.
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
| `lint` | Run gotest-specific linter checks |
| `refactor` | Source code refactoring tools (e.g. `toggle-focus`) |
| `discover` | Discover test suites and output JSON metadata |
| `prepare` | Start shared fixtures for debug (blocks until SIGTERM) |
| `version` | Print version information |
| `help` | Show help |

### Flags

| Flag | Effect |
|------|--------|
| `--debug` | Keep generated files after run |
| `--ci` | Fail if `F_` focus prefixes exist |
| `--spec` | Append spec summary after normal output |
| `--update-snapshots` | Regenerate snapshot files |
| `--format=<fmt>` | Output format for `spec` (terminal, md) |
| `--output=<path>` | Write formatted output to file |
| `--no-color` | Strip ANSI codes from terminal output |
| `--min=<pct>` | Fail if coverage below threshold (enables `-coverprofile`) |
| `--setup-timeout=<dur>` | Override fixture setup timeout |
| `--debounce=<dur>` | Debounce interval for watch mode (default 200ms) |
| `--input=<path>` | Input file for `spec` subcommand |

### Disambiguation

The first positional arg is checked against the known subcommand set.
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

All `go test` flags pass through verbatim.

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
- Unexported type names are silently ignored

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

1. **Parallel requires returning BeforeEach** — `SuiteConfig{Parallel: true}` with a void `BeforeEach` is an error (parallel methods need per-test isolation)
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

### Focus and Exclude

| Prefix | Effect |
|--------|--------|
| `F_` | **Focus** — only focused items run; all unfocused are skipped |
| `X_` | **Exclude** — always skipped, even if focused |
| *(none)* | Normal — runs unless something else is focused |

Rules:
1. If any suite has an `F_` prefix, all non-`F_` suites are skipped
2. If any test case within a suite has an `F_` prefix, all non-`F_` cases in that suite are skipped
3. `X_`-prefixed items are always skipped, regardless of focus state
4. Exclude takes precedence over focus
5. Focus/exclude is evaluated independently for `ptest` and `pxtest` suites

Skipped suites produce a `t.Skipf("test suite was excluded by user")` stub.

The `--ci` flag performs a static analysis scan before generation — no test execution needed:

```
$ gotest --ci ./...
FAIL: focus prefix detected — remove F_ before merging:
  pkg/user/user_test.go:12    type F_UserServiceTestSuite
  pkg/payment/pay_test.go:28  func (s *PaymentTestSuite) F_TestCharge
```

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
Each generated subtest calls `it.Parallel()` and coordinates via `sync.WaitGroup`.
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

When method-level parallelism is enabled, the generated code uses a `sync.WaitGroup` to ensure `AfterAll` waits for all parallel subtests to complete.
`wg.Done()` is `defer`-ed to prevent deadlocks on `t.Fatal()`.

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

Package fixture setup hooks receive `t.Context()` (cancelled when the test ends).
Cleanup hooks receive `context.Background()` (cleanup must proceed after test context cancellation).
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
  write to shared/state.json in overlay temp dir
  set GOTEST_SHARED_STATE_FILE env var for test process

Test process:
  read shared/state.json         → deserialize into struct (transferable fields populated, local fields zero-valued)
  sf.Hydrate(ctx)                → reconstructs local resources from transferred state
  ... run test suites ...
  sf.Dehydrate(ctx)              → cleans up local resources
```

**Validation at generation time:**

- Shared fixture types must not live in `internal/` packages.
  The setup subprocess compiles outside the module tree and cannot import `internal/` paths.
  Shared fixtures may freely depend on `internal/` packages — only the fixture type's own package path is restricted.
- If a transferable field's type has zero exported fields and does not implement `json.Marshaler`/`encoding.TextMarshaler`, the generator emits an error suggesting the field be handled in `Hydrate`
- If `Hydrate` exists without `Dehydrate`, the generator emits an error
- `Hydrate`/`Dehydrate` signatures must be `(ctx context.Context) error` with pointer receiver

### Configuration

Every fixture and suite runs with sensible defaults.
Optional marker methods override specific fields via overlay semantics — only non-zero fields replace the default.

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
    Retries      int           // per-test-case retry attempts
    FailFast     bool          // stop suite on first failure
    Parallel     bool          // method-level parallelism (requires returning BeforeEach)
}
```

#### Value Semantics

| Value | Meaning |
|-------|---------|
| `> 0` | Use this duration |
| `0`   | Keep default (field not overridden) |
| `< 0` | Explicitly disabled (no timeout) |

This applies to `Timeout`, `SetupTimeout`, and `RetryDelay`.
Boolean fields (`FailFast`, `Parallel`) only override to `true` — a false overlay does not reset a true base.

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

#### Presets

| Preset | Timeout | Retries | Use case |
|--------|---------|---------|----------|
| `DefaultFixtureConfig()` | 2 min | 0 | Standard fixtures |
| `ContainerFixtureConfig()` | 5 min | 1 (5s delay) | Testcontainers, image pulls |
| `DefaultSuiteConfig()` | 30s (+ 30s setup) | 0 | Unit/integration tests |
| `IntegrationSuiteConfig()` | 2 min (+ 5 min setup) | 0 | Heavier integration tests |

#### Overlay Functions

```go
func OverlayFixtureConfig(base *FixtureConfig, overlay FixtureConfig)
func OverlaySuiteConfig(base *SuiteConfig, overlay SuiteConfig)
```

Enables composable configuration — start with a preset, override individual fields:

```go
func (f *InfraFixture) FixtureConfig() gotest.FixtureConfig {
    cfg := gotest.ContainerFixtureConfig()
    if os.Getenv("CI") != "" {
        cfg.Timeout = 10 * time.Minute
        cfg.Retries = 2
    }
    return cfg
}
```

#### Generated Behavior

**Package fixtures:** The test harness always resolves `DefaultFixtureConfig()`, overlays when the marker is present, then uses the config to wrap `BeforeAll` in a retry loop with `context.WithTimeout` and wrap `AfterAll` cleanup with timeout.
Retry attempts are logged with attempt number.

**Shared fixtures:** The generated subprocess resolves `DefaultFixtureConfig()`, overlays when `SharedFixtureConfig()` is present, then wraps each SharedFixture's `BeforeAll(ctx)` in the same retry loop with `context.WithTimeout`.
After `BeforeAll`, transferable fields (determined by Hydrate-assignment analysis) are serialized to stdout as JSON.
`AfterAll(ctx)` gets timeout wrapping in the teardown handler.
In the test harness, the deserialized fixture is hydrated via `Hydrate(ctx)` if present, and `Dehydrate(ctx)` is deferred for cleanup.

**Suites:** The test harness always resolves `DefaultSuiteConfig()`, overlays when the marker is present, then wraps each test case with `NewTWithDeadline` when timeout > 0, and breaks the test case loop on first failure when `FailFast` is set.

**`NewTWithDeadline`:** Creates a `*gotest.T` with a context deadline.
`t.Context()` returns the deadline-aware context.
Existing `NewT` callers are unaffected.

#### Feature Interactions

- **Parallel suites:** `FailFast` checks run between subtests — in method-parallel suites, all parallel subtests complete before `FailFast` is evaluated.
  Suite-level parallelism does not affect `FailFast` (each suite's subtests are independent).
- **Focus/Exclude:** Config applies after focus/exclude filtering.
  Skipped suites get unchanged skip stubs.
- **Global `-timeout`:** `FixtureConfig.Timeout` is bounded by Go's global timeout via `context.WithTimeout` inheriting the parent's shorter deadline.
- **Nested fixtures:** Each level resolves config independently — no inheritance between fixture levels.
- **Hydrate/Dehydrate:** SharedFixture state is deserialized and hydrated before any test suites run.
  `Dehydrate` is deferred, running after all suites complete.
  `Hydrate` receives a context with the SharedFixture's configured timeout.
  `Dehydrate` receives `context.Background()`.

---

## Assertions

### Functional API

Type-safe generics with compile-time safety.
All functions accept any type implementing `testingT` (`Errorf()` + `FailNow()`) — works with `*gotest.T`, `*testing.T`, and `*gotest.R`.
`Helper()` is optional: when present, failures include the caller's file:line.

```go
// Equality — [V any] catches cross-type comparison at compile time
gotest.Equal[V any](t, expected, actual V, msgAndArgs ...any)
gotest.NotEqual[V any](t, expected, actual V, msgAndArgs ...any)

// Zero / Empty
gotest.Zero[V comparable](t, value V, msgAndArgs ...any)
gotest.NotZero[V comparable](t, value V, msgAndArgs ...any)
gotest.Empty(t, object any, msgAndArgs ...any)
gotest.NotEmpty(t, object any, msgAndArgs ...any)

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

Equality failures include a diff:

```
Equal failed:
  expected: map[string]int{"a": 1, "b": 2, "c": 3}
  actual:   map[string]int{"a": 1, "b": 5, "c": 3}
  diff:
    map[string]int{
    -   "b": 2,
    +   "b": 5,
    }
```

Zero external dependencies — uses `reflect.DeepEqual`, `cmp.Compare`, `fmt.Sprintf("%#v")`, and a minimal inline diff renderer.

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

---

## Async Assertions

### *gotest.R — Assertion Recorder

`*R` is an assertion recorder that captures assertion outcomes without propagating them to the test runner.
It satisfies the same `testingT` contract as `*testing.T` (`Errorf` + `FailNow`), making it the callback type for `Eventually` and `Consistently`.
All assertion functions work with `*R` just as they do with `*T` or `*testing.T`.

```go
type R struct { ... }
func (r *R) Errorf(format string, args ...any)
func (r *R) FailNow()
func (r *R) Helper()
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

- Snapshots stored in `testdata/__snapshots__/<TestName>.snap`
- First run: create snapshot, pass
- Subsequent runs: compare, fail with diff on mismatch
- Update: `gotest --update-snapshots ./...`

---

## Scaffolding

```
$ gotest scaffold ./pkg/user.UserService
```

Generates a test suite skeleton with `BeforeEach`, per-method `Test*` stubs, and method signatures as comments.
Methods returning `error` get happy-path and error-case stubs.

Interface scaffolding generates a generic contract suite:

```
$ gotest scaffold --contract io.ReadCloser
```

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
4. Removes `suite.Run` boilerplate and `testify/suite` import

Handles the 90% case; leaves `// TODO: manual review` comments for edge cases.

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
    ~ hard-deletes after 30 days — SKIPPED

2 suites, 5 behaviors: 4 passed, 0 failed, 1 skipped
```

Internally runs `go test -json`, parses the event stream, reconstructs the suite→method→When/It hierarchy from `/`-separated test paths, and strips Go naming conventions for display.

Output formats:

```bash
gotest spec ./...                                    # terminal (color tree)
gotest spec ./... --no-color                         # terminal (plain)
gotest spec ./... --format=md --output=behavior.md   # markdown
gotest ./... -v --spec                               # append after normal output
```

---

## Watch Mode

```bash
gotest watch ./... -v
gotest watch ./... --spec
```

1. Initial run: full generate → test → cleanup cycle
2. Watch filesystem for `.go` file changes (via `fsnotify`)
3. On change: re-run for affected packages only
4. 200ms debounce on rapid changes
5. Terminal clear between runs

Focus integration: `F_`-prefixed suites during watch mode create a tight feedback loop — only focused tests re-run on each save.

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

Walks directories and removes files matching `ƒƒ_p(x)?suite_test.go`.

### Generated Files

Per package directory, up to two files:

| File | Purpose |
|------|---------|
| `ƒƒ_psuite_test.go` | Same-package (white-box) test suites |
| `ƒƒ_pxsuite_test.go` | External-package (black-box) test suites |

The `ƒƒ` Unicode prefix prevents naming collisions with user code.
Files contain a `// Code generated` header.

### Generated Structure

For each suite, the generated code creates a wrapper struct with no-op fallbacks for unimplemented hooks, then a `func Test*` that wires lifecycle and inline `t.Run` blocks:

```go
type ƒƒ_GOTEST_MyTestSuite struct { MyTestSuite }

func TestMyTestSuite(t *testing.T) {
    s := &ƒƒ_GOTEST_MyTestSuite{}
    ƒcfg := gotest.DefaultSuiteConfig()

    ƒsetupT := gotest.NewT(t)
    if ƒcfg.SetupTimeout > 0 {
        ƒsetupT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
    }
    t.Cleanup(func() {
        ƒteardownT := gotest.NewT(t)
        if ƒcfg.SetupTimeout > 0 {
            ƒteardownT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
        }
        s.AfterAll(ƒteardownT)
    })
    s.BeforeAll(ƒsetupT)

    t.Run("TestSomething", func(it *testing.T) {
        ttt := gotest.NewT(it)
        if ƒcfg.Timeout > 0 {
            ttt = gotest.NewTWithDeadline(it, ƒcfg.Timeout)
        }
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        ƒƒ_GOTEST_exec(s.TestSomething, ttt)
    })
}
```

---

## Linter

Available as a subcommand and as a standalone binary, built on `go/analysis` and compatible with `golangci-lint`:

```bash
gotest lint ./...                                          # subcommand
go run github.com/mvrahden/go-test/cmd/gotest-lint ./...   # standalone
```

Detects:
- **Lifecycle hook typos:** `BeforAll`, `AfterEeach` — methods on suite structs within Levenshtein distance ≤ 2 of known hooks
- **Value receivers:** Suite methods with value receivers instead of pointer receivers
- **Missing lifecycle pair:** `BeforeAll` without `AfterAll` — resources may leak
- **Committed focus prefixes:** `F_` prefix on types or methods
- **Orphaned generated files:** `ƒƒ_*` files checked into version control
- **Stdlib test functions:** `func TestX(t *testing.T)` in packages with suites — likely meant to be a suite method
- **Testify imports:** `testify/suite` imports in packages using gotest — migration incomplete
- **Poll scope errors:** Assertions inside `Eventually`/`Consistently` callbacks using the outer `t` instead of the `poll` parameter

---

## CI Integration

```yaml
- uses: ./.github/actions/setup-gotest  # local composite action
- run: gotest --ci ./... -v -race -coverprofile=coverage.out
- run: gotest spec ./... --format=md --output=behavior-spec.md
```

Exit codes match `go test`: 0 = pass, 1 = test failure, 2 = build error.

The `--ci` flag fails the run when any `F_` (focus) prefix is committed, preventing accidental focus leaks in CI.
The local `setup-gotest` composite action installs the binary via `go install`.

---

## Coverage Aggregation

The Go coverage profile is the single source of truth at every level of aggregation.
No filesystem scanning, line counting heuristics, or mixed data sources.

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
4. In method-parallel suites (`Parallel: true`), `wg.Wait()` completes before `AfterAll`
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
They add a layer on top; they never suppress or replace the underlying output.

---

## Architecture

### Package Graph

```
cmd/gotest/                  CLI entrypoint, subcommands, arg handling
  └── internal/gotestrunner/   Suite generation I/O, go test execution, overlay
        └── internal/gotestgen/   Package loading, collection, fixture resolution, rendering
              └── internal/gotestast/   AST analysis, spec model, regex classification

cmd/gotest-lint/             Standalone linter binary (singlechecker)
  └── internal/lint/           go/analysis analyzer

internal/config/             gotest.yaml configuration loading
internal/gotestspec/         Spec tree builder and renderers (terminal, markdown)
internal/scaffold/           Type-to-suite skeleton generator
internal/migrate/            testify/suite AST transformer
internal/refactor/           AST refactoring tools (focus/exclude toggle)

pkg/gotest/                  User-facing API (T, R, assertions, Each, Eventually, Consistently, MatchSnapshot)
  └── internal/assert/         Core assertion implementation (pure stdlib)
  └── internal/snapfile/       Snapshot file I/O and diffing

about/                       Build metadata, file naming constants
```

### Code Generation Pipeline

| Stage | Input | Output |
|-------|-------|--------|
| **Load** | Package pattern (e.g., `./...`) | `[]*packages.Package` with syntax, types, module info |
| **Collect** | Package AST | `TestSuiteSpecSet` — suites with attached methods, local fixture specs |
| **Reduce** | `TestSuiteSpecSet` | Effective set after focus/exclude applied |
| **Resolve** | Effective suites + local fixtures | `ResolveResult` — fixture trees, shared fixture info, suite→fixture bindings |
| **Render** | Reduced spec + resolve result | Formatted Go source bytes |

Collection is a two-pass AST traversal: find type declarations matching suite patterns, then attach methods by matching receiver types.
Resolution is demand-driven: it starts from targeted suites and walks the Go type graph recursively (via `types.Named`, `types.Struct`, `types.MethodSet`) to discover all required fixtures, including cross-package dependencies.

### Key Invariant

The pipeline is always: **static analysis → code generation → standard `go test`**.
No runtime component grows beyond the thin `gotest.T` wrapper.
If a feature can't be implemented as either (a) generated code, (b) a method on `gotest.T`, or (c) post-processing of `go test -json`, it doesn't belong in this project.

---

## Known Limitations

1. **Generic aliases in `pxtest`:** Go does not allow defining methods on aliases of types from other packages.
   Generic suite aliases only work in same-package tests.

2. **`go.work` required for cross-module tests:** The generator golden tests require `go.work` (`go work init . && go work use ./examples`).
   Tests skip gracefully when absent.

3. **No incremental generation:** The tool regenerates all suite files on every run.
   There is no staleness detection.

4. **Hydrate method walking depth:** The generator follows receiver method calls from `Hydrate` one level deep to classify local fields.
   Assignments hidden behind two or more levels of indirection are not detected.
   Opaque types (zero exported fields) are unaffected — they serialize harmlessly as `{}` and `Hydrate` overwrites the value.
