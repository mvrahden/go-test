# gotest Specification

Go tests that write themselves, organize themselves, and explain themselves.

`gotest` closes the gap between `func TestX(t *testing.T)` and a well-organized test suite through code generation. Developers write structs, name them well, and the tool handles the rest. No runtime dependencies. No reflection. No lock-in. Just standard Go tests with lifecycle management and structured organization.

A Go developer should be able to `go install` this tool and immediately write better-organized tests without learning a framework. The naming conventions are the API. The generated code is the implementation. The output is `go test` output.

---

## Design Principles

Ranked. When they conflict, higher-ranked principles win.

### 1. Standard Go output, always

Every generated test is a `func Test*(t *testing.T)`. Every line of output is standard `go test` output. Every CI system, IDE, coverage tool, and profiler works unchanged.

### 2. The naming IS the API

No config files. No struct tags. No annotations. No registration calls. A developer reads the naming conventions once and never opens documentation again.

### 3. Zero runtime cost

The tool generates code at build time and cleans it up after. At test execution time, there is no `go-test` code in the call stack (except the thin `gotest.T` wrapper). No reflection, no interface dispatch, no type assertions. The generated code is what a careful developer would write by hand.

### 4. Invisible until needed

A developer who has never heard of `go-test` can read a test suite struct and understand what it does. A developer who runs `go test` directly (without the CLI) gets a compilation error for the missing generated file — not silent wrong behavior.

### 5. Adopt incrementally, eject freely

Existing `func Test*` tests coexist with suites in the same package. Removing the tool means deleting suite structs and writing the equivalent `func Test*` functions — the generated code shows exactly what to write.

---

## Conceptual Model

Test suites are behavioral specifications. Every level of the test hierarchy maps to a specification concept:

```
struct  = Subject     "UserService"
method  = Capability  "Create"
When()  = Context     "when email is valid"
It()    = Behavior    "creates the user"
Each()  = Variants    "standard format", "missing @", "empty string"
```

The naming conventions at the struct/method level and the string descriptions at the `It`/`When` level together form a complete behavioral specification. The tool generates the bridge code (lifecycle, parallel coordination, focus/exclude) and can render the full specification in human-readable form.

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

### Disambiguation

The first positional arg is checked against the known subcommand set. If it matches, it's consumed. Otherwise, it's a package pattern. `gotest ./watch` tests the `watch` package; `gotest watch` enters watch mode.

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

A test suite is a Go struct whose name ends in `TestSuite` (or `TestSuiteParallel` for parallel suites):

```go
type MyTestSuite struct {
    sut *MyService
}
```

Requirements:
- Name matches `^(?!ƒƒ_GOTEST_|_)(?:X_|F_)?.+(TestSuite|TestSuiteParallel)$`
- Methods use pointer receivers (`*MyTestSuite`, not `MyTestSuite`)
- Each suite must be its own `type` declaration (not block-style)
- Unexported type names are silently ignored

### Lifecycle Hooks

All hooks are optional. Unimplemented hooks become no-ops in the generated code.

| Method | Signature | Semantics |
|--------|-----------|-----------|
| `BeforeAll` | `func (s *Suite) BeforeAll(t *gotest.T)` | Once before the first test case |
| `AfterAll` | `func (s *Suite) AfterAll(t *gotest.T)` | Once after the last test case (via `t.Cleanup`) |
| `BeforeEach` | `func (s *Suite) BeforeEach(t *gotest.T)` | Before each test case |
| `AfterEach` | `func (s *Suite) AfterEach(t *gotest.T)` | After each test case (via `defer`) |

Execution order:

```
BeforeAll
├── BeforeEach → Test A → AfterEach (deferred)
├── BeforeEach → Test B → AfterEach (deferred)
AfterAll (via t.Cleanup — LIFO, runs after all subtests)
```

`AfterAll` is registered via `t.Cleanup` before `BeforeAll` runs, ensuring it executes even if `BeforeAll` registers its own cleanup functions (LIFO ordering). `AfterEach` is `defer`-ed, ensuring it runs even when `t.Fatal()` triggers `runtime.Goexit()`.

Hooks accept either `*gotest.T` or `*testing.T`.

### Test Cases

Any exported method matching `^(?:X_|F_)?(Test(?!Parallel)|TestParallel).+$` is a test case. Each becomes a `t.Run` subtest in the generated code.

```go
func (s *Suite) TestSomething(t *gotest.T)   {} // sequential test
func (s *Suite) TestParallelFoo(t *gotest.T) {} // parallel test
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

### Parallel Execution

**Suite-level:** Struct name ends in `TestSuiteParallel`. The generated `func Test*` calls `t.Parallel()`.

**Test-case-level:** Method name starts with `TestParallel`. The generated subtest calls `it.Parallel()` and coordinates via `sync.WaitGroup`.

```go
type MyTestSuiteParallel struct{}

func (s *MyTestSuiteParallel) TestParallelAlpha(t *gotest.T)  {} // parallel
func (s *MyTestSuiteParallel) TestParallelBeta(t *gotest.T)   {} // parallel
func (s *MyTestSuiteParallel) TestSequentialGamma(t *gotest.T) {} // sequential
```

When parallel test cases exist, the generated code uses a `sync.WaitGroup` to ensure `AfterAll` waits for all parallel subtests to complete. `wg.Done()` is `defer`-ed to prevent deadlocks on `t.Fatal()`.

### Generic Suites

Generic struct definitions are not code generation targets, but their instantiated type aliases are:

```go
type GenericTestSuite[T any] struct { value T }

func (s *GenericTestSuite[T]) TestSomething(t *gotest.T) {}

type StringTestSuite = GenericTestSuite[string]
type IntTestSuite    = GenericTestSuite[int]
```

Each alias produces an independent test suite. Generic aliases only work in same-package tests (`ptest`), not `pxtest`.

### Fixtures

Structs ending in `Fixture` are package fixtures. Structs ending in `SharedFixture` are cross-package shared fixtures. Fixture hooks receive `context.Context` and return `error`:

```go
func (f *MyFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *MyFixture) AfterAll(ctx context.Context) error   { return nil }
```

Setup hooks receive `t.Context()` (cancelled when the test ends). Cleanup hooks receive `context.Background()` (cleanup must proceed after test context cancellation).

Test suites embed fixtures via pointer embedding:

```go
type BatchTestSuite struct {
    *E2ESetupFixture
}
```

Fixtures nest — a root fixture's hooks run first, wrapping the child's:

```
InfraFixture.BeforeAll
  APIFixture.BeforeAll
    Suite.BeforeAll
      Suite.BeforeEach → Test → Suite.AfterEach
    Suite.AfterAll
  APIFixture.AfterAll
InfraFixture.AfterAll
```

---

## Assertions

### Functional API

Type-safe generics with compile-time safety. All functions accept any type implementing `testingT` (`Helper()` + `Errorf()` + `FailNow()`) — works with both `*gotest.T` and `*testing.T`.

```go
// Equality — [T any] catches cross-type comparison at compile time
gotest.Equal[T any](t, expected, actual T, msgAndArgs ...any)
gotest.NotEqual[T any](t, expected, actual T, msgAndArgs ...any)

// Zero / Empty
gotest.Zero[T comparable](t, value T, msgAndArgs ...any)
gotest.NotZero[T comparable](t, value T, msgAndArgs ...any)
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
gotest.ElementsMatch[T comparable](t, listA, listB []T, msgAndArgs ...any)
gotest.Subset[T comparable](t, list, subset []T, msgAndArgs ...any)

// Comparison — [T cmp.Ordered] prevents comparing incomparable types
gotest.Greater[T cmp.Ordered](t, a, b T, msgAndArgs ...any)
gotest.GreaterOrEqual[T cmp.Ordered](t, a, b T, msgAndArgs ...any)
gotest.Less[T cmp.Ordered](t, a, b T, msgAndArgs ...any)
gotest.LessOrEqual[T cmp.Ordered](t, a, b T, msgAndArgs ...any)

// String / Regex
gotest.Regexp[P regexpPattern](t, rx P, str string, msgAndArgs ...any)

// Numeric
gotest.InDelta[T numeric](t, expected, actual T, delta float64, msgAndArgs ...any)

// Serialization — accepts string, []byte, json.RawMessage, io.Reader, or marshalable
gotest.JSONEq(t, expected, actual any, msgAndArgs ...any)

// Time
gotest.TimeWithin(t, expected, actual time.Time, tolerance time.Duration, msgAndArgs ...any)
gotest.TimeIsNow(t, ts time.Time, tolerance time.Duration, msgAndArgs ...any)

// Panic
gotest.Panics(t, f func(), msgAndArgs ...any) any

// Unwrap — panics on failure (no t parameter, enables multi-return expansion)
gotest.Must[T any](val T, ok any) T
```

All functions call `t.Helper()` so failures report the caller's file:line. Equality failures include a diff:

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

### Fluent API

Discoverable via autocomplete. Delegates to the functional layer. Accepts `any` (runtime type checking, not compile-time):

```go
t.Assert(result).Equal(expected)
t.Assert(items).HasLength(3)
t.Assert(err).NoError()
t.Assert(ok).IsTrue()
```

---

## BDD Vocabulary

### t.When() / t.It()

`When` groups context. `It` specifies behavior. Both map to `t.Run` under the hood — the distinction is purely semantic.

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

### t.Each()

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

Callback API:

```go
t.Each(cases, func(it *gotest.T, tc Case) {
    gotest.Equal(it, tc.Want, parse(tc.Input))
})
```

Uses `Desc` or `Name` field for the subtest name, falls back to `#0`, `#1`, etc.

---

## Async Assertions

**Boolean polling** (functional API):

```go
gotest.Eventually(t, func() bool { return s.svc.IsReady() }, 5*time.Second, 100*time.Millisecond)
gotest.Consistently(t, func() bool { return cache.IsValid() }, 2*time.Second, 50*time.Millisecond)
```

**Rich assertion polling** (methods on `*gotest.T`):

```go
t.Eventually(5*time.Second, 100*time.Millisecond, func(poll *gotest.T) {
    result, err := s.store.Get("key")
    gotest.NoError(poll, err)
    gotest.Equal(poll, result.Status, "completed")
})

t.Consistently(2*time.Second, 100*time.Millisecond, func(poll *gotest.T) {
    gotest.Equal(poll, s.counter.Value(), 0)
})
```

The `poll *gotest.T` is a collecting wrapper — assertion failures during intermediate polls are captured but not propagated. On timeout, the last poll's assertion failures are reported. `Consistently` fails on first `false` poll and reports which poll failed.

---

## Snapshot Testing

```go
t.MatchSnapshot(result)               // auto-named from test
t.MatchSnapshot(result, "variant")    // custom snapshot name
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

Generates a test suite skeleton with `BeforeEach`, per-method `Test*` stubs, and method signatures as comments. Methods returning `error` get happy-path and error-case stubs.

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

Writes generated files directly to package directories. Use case: `//go:generate gotest generate ./...` for checked-in generated files.

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

The `ƒƒ` Unicode prefix prevents naming collisions with user code. Files contain a `// Code generated` header.

### Generated Structure

For each suite, the generated code creates a wrapper struct with no-op fallbacks for unimplemented hooks, then a `func Test*` that wires lifecycle and test cases:

```go
type ƒƒ_GOTEST_MyTestSuite struct { MyTestSuite }

func TestMyTestSuite(t *testing.T) {
    s := &ƒƒ_GOTEST_MyTestSuite{}
    // ...
    tt := gotest.NewT(t)
    t.Cleanup(func() { s.AfterAll(tt) })
    s.BeforeAll(tt)
    for _, tc := range testCases { tc(tt) }
}
```

---

## Linter

`gotest-lint` is a standalone binary built on `go/analysis`, compatible with `golangci-lint`:

```bash
gotest-lint ./...
```

Detects:
- **Lifecycle hook typos:** `BeforAll`, `AfterEeach` — methods on suite structs within Levenshtein distance ≤ 2 of known hooks
- **Value receivers:** Suite methods with value receivers instead of pointer receivers
- **Missing lifecycle pair:** `BeforeAll` without `AfterAll` — resources may leak
- **Committed focus prefixes:** `F_` prefix on types or methods
- **Orphaned generated files:** `ƒƒ_*` files checked into version control

---

## CI Integration

```yaml
- uses: mvrahden/setup-gotest@v1
- run: gotest --ci ./... -v -race -coverprofile=coverage.out
- run: gotest spec ./... --format=md --output=behavior-spec.md
```

Exit codes match `go test`: 0 = pass, 1 = test failure, 2 = build error.

The `setup-gotest` composite action installs the binary via `go install`.

---

## Advanced Patterns

### Nested Suites via Embedding

A suite embedding another suite inherits its lifecycle hooks. The generator chains parent/child hooks:

```
ParentTestSuite.BeforeAll
└── ChildTestSuite
    ├── ParentTestSuite.BeforeEach (if defined)
    │   └── ChildTestSuite.BeforeEach
    │       └── Test
    │       └── ChildTestSuite.AfterEach
    │   └── ParentTestSuite.AfterEach
ParentTestSuite.AfterAll
```

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
4. In parallel suites, `wg.Wait()` completes before `AfterAll`
5. `t.Fatal()` in `BeforeAll` skips the entire suite
6. `t.Skip()` in `BeforeAll` marks the suite as skipped

---

## Non-Goals

### Test dependency ordering

Tests that depend on other tests are brittle. Each test sets up its own preconditions via `BeforeEach`.

### Mocking framework

Mocking is orthogonal to test organization. `gomock`, `mockery`, `moq`, and counterfeiter all work inside suites unchanged.

### Decorator / annotation syntax

Go doesn't have decorators. Naming conventions are grepped, autocompleted, and understood at a glance. Struct tags are hidden in backtick strings.

### Runtime suite registration

`suite.Run(t, new(MySuite))` is testify's approach. The entire point of `go-test` is to generate that boilerplate.

### Cross-package suite inheritance

Breaks Go's package isolation model. Cross-package sharing uses helper functions, not suite embedding.

### Replacing `go test` output

The `spec` subcommand and `--spec` flag are post-processing views over `go test -json` data. They add a layer on top; they never suppress or replace the underlying output.

---

## Architecture

### Package Graph

```
cmd/gotest/                  CLI entrypoint, subcommands, arg handling
  └── internal/gotestrunner/   Suite generation I/O, go test execution, overlay
        └── internal/cmd/testgen/   Generation orchestration
              └── internal/gotestgen/   Package loading, collection, rendering
                    └── internal/gotestast/   AST analysis, spec model, regex classification

cmd/gotest-lint/             Standalone linter binary (singlechecker)
  └── internal/lint/           go/analysis analyzer

internal/gotestspec/         Spec tree builder and renderers (terminal, markdown)
internal/scaffold/           Type-to-suite skeleton generator
internal/migrate/            testify/suite AST transformer

pkg/gotest/                  User-facing API (T, Assert, It, When, Each, Eventually, MatchSnapshot)
  └── internal/assert/         Core assertion implementation (~300 lines, pure stdlib)

about/                       Build metadata, file naming constants
```

### Code Generation Pipeline

| Stage | Input | Output |
|-------|-------|--------|
| **Load** | Package pattern (e.g., `./...`) | `[]*packages.Package` with syntax, types, module info |
| **Collect** | Package AST | `TestSuiteSpecSet` — suites with attached methods |
| **Reduce** | `TestSuiteSpecSet` | Effective set after focus/exclude applied |
| **Render** | Reduced spec | Formatted Go source bytes |

Collection is a two-pass AST traversal: find type declarations matching suite patterns, then attach methods by matching receiver types.

### Key Invariant

The pipeline is always: **static analysis → code generation → standard `go test`**. No runtime component grows beyond the thin `gotest.T` wrapper. If a feature can't be implemented as either (a) generated code, (b) a method on `gotest.T`, or (c) post-processing of `go test -json`, it doesn't belong in this project.

---

## Known Limitations

1. **Generic aliases in `pxtest`:** Go does not allow defining methods on aliases of types from other packages. Generic suite aliases only work in same-package tests.

2. **`go.work` required for cross-module tests:** The generator golden tests require `go.work` (`go work init . && go work use ./examples`). Tests skip gracefully when absent.

3. **No incremental generation:** The tool regenerates all suite files on every run. There is no staleness detection.
