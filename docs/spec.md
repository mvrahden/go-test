# go-test Specification

> **Deprecated:** This document reflects an earlier version of the project. See [superspec.md](superspec.md) for the canonical reference.

## Purpose

`gotest` is a code-generation tool that adds xUnit-style test suite semantics to Go's testing framework. It operates as a transparent wrapper around `go test`: users define test suites as Go structs following naming conventions, and the tool generates the standard `func Test*(t *testing.T)` entry points that Go requires — then cleans them up after the run.

The result is structured, lifecycle-managed test suites that produce standard `go test` output, work with all existing Go tooling, and require zero runtime dependencies.

## Design Principles

1. **Transparency.** The tool is invisible at runtime. Generated code is standard Go. Test output is standard `go test` output. All existing tools (IDEs, CI systems, coverage reporters) work unchanged.

2. **Convention over configuration.** Suite detection, lifecycle hooks, focus/exclude behavior, and parallelism are all driven by naming patterns. No annotations, no config files, no struct tags.

3. **Ephemeral generation.** Generated files exist only during the test run. Cleanup is guaranteed via `defer` and signal handling. The working tree stays clean.

4. **Zero reflection.** Unlike testify/suite, there is no runtime reflection. The generated code contains concrete, typed method calls. This provides compile-time safety and zero overhead.

5. **Full `go test` compatibility.** All `go test` flags pass through verbatim. The tool never interprets or modifies `go test` arguments. It only processes its own `-ƒƒ.*` flags.

6. **Co-existence.** Suite-based tests and traditional `func Test*(t *testing.T)` functions coexist in the same package with no interference.

---

## User API

### Test Suite Definition

A test suite is a Go struct whose name ends in `TestSuite` (or `TestSuiteParallel` for parallel suites).

```go
type MyTestSuite struct {
    sut *MyService
}
```

**Requirements:**
- The struct name must match the pattern `^(?!ƒƒ_GOTEST_|_)(?:X_|F_)?.+(TestSuite|TestSuiteParallel)$`
- Methods must use pointer receivers (`*MyTestSuite`, not `MyTestSuite`)
- All lifecycle hooks and test methods accept a single parameter: `*gotest.T`

**Disallowed:**
- Value-type receivers (compile-time error from generator)
- Unexported type names (silently ignored — Go convention)
- Block-style type declarations (`type ( A struct{}; B struct{} )`) — each suite must be its own `type` declaration

### Generic Test Suites

Generic struct definitions are not code generation targets themselves, but their instantiated type aliases are:

```go
type GenericTestSuite[T any] struct {
    value T
}

func (s *GenericTestSuite[T]) BeforeEach(t *gotest.T) { /* ... */ }
func (s *GenericTestSuite[T]) TestSomething(t *gotest.T) { /* ... */ }

// These aliases ARE generation targets:
type StringTestSuite = GenericTestSuite[string]
type IntTestSuite    = GenericTestSuite[int]
```

Each alias produces an independent test suite with the full method set of the underlying generic type. The generated test functions are `TestStringTestSuite` and `TestIntTestSuite`.

**Constraint:** Generic aliases only work in same-package tests (`ptest`), not external-package tests (`pxtest`), because Go does not allow defining methods on aliases of types from other packages.

### Lifecycle Hooks

All hooks are optional. Unimplemented hooks become no-ops in the generated code.

| Method | Signature | Semantics |
|--------|-----------|-----------|
| `BeforeAll` | `func (s *Suite) BeforeAll(t *gotest.T)` | Runs once before the first test case |
| `AfterAll` | `func (s *Suite) AfterAll(t *gotest.T)` | Runs once after the last test case (via `t.Cleanup`) |
| `BeforeEach` | `func (s *Suite) BeforeEach(t *gotest.T)` | Runs before each test case |
| `AfterEach` | `func (s *Suite) AfterEach(t *gotest.T)` | Runs after each test case (via `defer`) |

**Execution order for a suite with tests A and B:**

```
BeforeAll
├── BeforeEach → Test A → AfterEach (deferred)
├── BeforeEach → Test B → AfterEach (deferred)
AfterAll (via t.Cleanup — LIFO, runs after all subtests)
```

`AfterAll` is registered via `t.Cleanup` before `BeforeAll` runs, ensuring it executes even if `BeforeAll` registers its own cleanup functions (LIFO ordering).

`AfterEach` is `defer`-ed, ensuring it runs even when `t.Fatal()` triggers `runtime.Goexit()`.

### Test Cases

Any exported method on the suite struct matching `^(?:X_|F_)?(Test(?!Parallel)|TestParallel).+$` is a test case.

```go
func (s *Suite) TestSomething(t *gotest.T)     { /* sequential test */ }
func (s *Suite) TestParallelFoo(t *gotest.T)   { /* parallel test */ }
func (s *Suite) TestSomethingAsync(t *gotest.T, done func()) { /* async test */ }
```

Each test case becomes a `t.Run` subtest in the generated code.

### Focus and Exclude

Naming prefixes control which suites and test cases run, inspired by Jest/Jasmine:

| Prefix | Effect | Scope |
|--------|--------|-------|
| `F_` | **Focus** — only focused items run; all unfocused are skipped | Suite or test case |
| `X_` | **Exclude** — always skipped, even if focused | Suite or test case |
| *(none)* | Normal — runs unless something else is focused | Suite or test case |

**Rules:**
1. If any suite has an `F_` prefix, all non-`F_` suites are skipped
2. If any test case within a suite has an `F_` prefix, all non-`F_` cases in that suite are skipped
3. `X_`-prefixed items are always skipped, regardless of focus state
4. Exclude takes precedence over focus
5. Focus/exclude is evaluated independently for `ptest` and `pxtest` suites

Skipped suites produce a `t.Skipf("test suite was excluded by user")` stub so they appear in test output.

```go
type F_FocusedTestSuite struct{}    // only this suite runs
type NormalTestSuite struct{}       // skipped (unfocused)
type X_ExcludedTestSuite struct{}   // skipped (excluded)

func (s *F_FocusedTestSuite) TestAlpha(t *gotest.T) {}      // runs
func (s *F_FocusedTestSuite) F_TestBeta(t *gotest.T) {}     // if F_ tests exist, only this runs
func (s *F_FocusedTestSuite) X_TestGamma(t *gotest.T) {}    // always skipped
```

### Parallel Execution

Two mechanisms:

**Suite-level parallelism:** Struct name ends in `TestSuiteParallel`. The generated `func Test*` calls `t.Parallel()`.

**Test-case-level parallelism:** Method name starts with `TestParallel`. The generated subtest calls `it.Parallel()` and coordinates via `sync.WaitGroup`.

```go
type MyTestSuiteParallel struct{}

func (s *MyTestSuiteParallel) TestParallelAlpha(t *gotest.T) {} // runs in parallel
func (s *MyTestSuiteParallel) TestParallelBeta(t *gotest.T)  {} // runs in parallel
func (s *MyTestSuiteParallel) TestSequentialGamma(t *gotest.T) {} // runs sequentially
```

**WaitGroup contract:** When parallel test cases exist, the generated code uses a `sync.WaitGroup` to ensure `AfterAll` (registered via `t.Cleanup`) waits for all parallel subtests to complete. `wg.Done()` is `defer`-ed to prevent deadlocks when tests fail via `t.Fatal()`.

### Async Test Cases

Method names ending in `Async` receive a `done func()` callback as a second parameter:

```go
func (s *Suite) TestSomethingAsync(t *gotest.T, done func()) {
    go func() {
        defer done()
        // async test logic
    }()
}
```

### BDD-Style Nesting

`gotest.T` provides `It()` for Jasmine/RSpec-style nested subtests:

```go
func (s *Suite) TestFeature(t *gotest.T) {
    t.It("succeeds when input is valid", func(it *gotest.T) {
        it.Assert(result).IsEqualTo(expected)
    })
}
```

### Fluent Assertions

`gotest.T` provides `Assert(v)` returning a fluent assertion context:

| Method | Description | Status |
|--------|-------------|--------|
| `IsTrue()` | Boolean true assertion | Complete |
| `IsFalse()` | Boolean false assertion | Complete |
| `IsEqualTo(v)` | Deep equality via `reflect.DeepEqual` | Complete |
| `IsZero()` | Zero-value check | Complete |
| `IsEmpty()` | Length == 0 for strings, maps, slices, arrays, channels | Complete |
| `HasLength(n)` | Exact length check | Complete |
| `HasCapacity(n)` | Exact capacity check for slices, arrays, channels | Complete |
| `Contains(v)` | Element containment | Partial (string case is a no-op) |
| `ContainsAll(v...)` | Multi-element containment | Stub (panics) |
| `ContainsAny(v...)` | Any-element containment | Stub (panics) |

Assertion failures route through `t.Fatalf`, immediately failing the current subtest. A `{{FMT}}` token system produces human-readable error messages that render type names instead of pointer addresses.

The `require` sub-package provides type-safe generic assertions as a separate API alongside the fluent assertions.

---

## CLI Interface

```
testsuite [ƒƒ-flags...] [go-test-args...]
```

The tool splits its own flags (prefixed `-ƒƒ.`) from everything else, which is passed to `go test` verbatim.

### Flags

| Flag | Effect |
|------|--------|
| `-ƒƒ.internal.debug` | Skip cleanup — leave generated files for inspection |
| `-ƒƒ.clean` | Remove orphaned generated files and exit (no test run) |

### Package Discovery

Package patterns are extracted from `go test` args using a heuristic: non-flag tokens (not starting with `-`) that look like package patterns (start with `.` or `/`, or contain `/`). If no patterns are found, defaults to `.`.

The `-args` token stops pattern extraction (consistent with `go test` semantics).

### Execution Pipeline

```
main()
├── 1. Split ƒƒ-flags from go-test-args
├── 2. Extract package patterns from go-test-args
├── 3. For each pattern: generate suite files via packages.Load + AST analysis
├── 4. defer: cleanup generated files (runs on signal, panic, any exit path)
├── 5. signal.NotifyContext for SIGINT/SIGTERM
├── 6. Exec `go test` with original go-test-args, streaming stdout/stderr
└── 7. Exit with `go test`'s exit code
```

### Clean Mode

`testsuite -ƒƒ.clean [paths...]` walks the specified directories (stripping `/...` suffixes) and removes any files matching the generated file regex `ƒƒ_p(x)?suite_test\.go$`.

---

## Code Generation

### Generated Files

Per package directory, up to two files are generated:

| File | Purpose |
|------|---------|
| `ƒƒ_psuite_test.go` | Same-package (white-box) test suites |
| `ƒƒ_pxsuite_test.go` | External-package (black-box) test suites |

The `ƒƒ` Unicode prefix prevents naming collisions with user code.

Files are written with `0644` permissions and contain a `// Code generated` header that signals to editors and tools that they are machine-generated.

### Generated Structure Per Suite

For each effective suite (after focus/exclude reduction):

```go
type ƒƒ_GOTEST_MyTestSuite struct {
    MyTestSuite
}

func (ts *ƒƒ_GOTEST_MyTestSuite) BeforeAll(it *gotest.T)  { ts.MyTestSuite.BeforeAll(it) }
func (ts *ƒƒ_GOTEST_MyTestSuite) AfterAll(it *gotest.T)   { /* no-op if not implemented */ }
func (ts *ƒƒ_GOTEST_MyTestSuite) BeforeEach(it *gotest.T) { ts.MyTestSuite.BeforeEach(it) }
func (ts *ƒƒ_GOTEST_MyTestSuite) AfterEach(it *gotest.T)  { /* no-op if not implemented */ }

func TestMyTestSuite(t *testing.T) {
    s := &ƒƒ_GOTEST_MyTestSuite{}

    newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
        return func(tt *gotest.T) {
            t := tt.T()
            t.Run(desc, func(it *testing.T) {
                ttt := gotest.NewT(it)
                defer s.AfterEach(ttt)
                s.BeforeEach(ttt)
                testFn(ttt)
            })
        }
    }

    testCases := []gotest.TestCase{
        newTestCase("TestFoo", s.TestFoo),
        newTestCase("TestBar", s.TestBar),
    }

    tt := gotest.NewT(t)
    t.Cleanup(func() { s.AfterAll(tt) })
    s.BeforeAll(tt)
    for _, tc := range testCases { tc(tt) }
}
```

The wrapper struct exists to provide explicit no-op fallbacks for unimplemented hooks, avoiding the need for interface checks at runtime.

### Skipped Suites

Suites excluded by focus/exclude rules generate a stub:

```go
func TestExcludedSuite(t *testing.T) {
    t.Skipf("test suite was excluded by user")
}
```

---

## Internal Architecture

### Package Dependency Graph

```
cmd/testsuite/          CLI entrypoint, arg handling, cleanup
  └── internal/gotestrunner/   Suite generation I/O, go test execution
        └── internal/cmd/testgen/   Generation orchestration
              └── internal/gotestgen/   Package loading, collection, rendering
                    ├── internal/gotestast/   AST analysis, spec model, regex classification
                    └── static/              Template files (gotest.suites.tpl, header.go.tpl)

pkg/gotest/             User-facing test wrapper (T, TestCase, It, Assert)
  └── pkg/gotest/internal/assert/   Fluent assertion implementation

about/                  Build metadata, file naming constants, regex patterns
```

### Code Generation Pipeline

| Stage | Package | Input | Output |
|-------|---------|-------|--------|
| **Load** | `gotestgen` | Package pattern (e.g., `./...`) | `[]*packages.Package` with syntax, types, module info |
| **Collect** | `gotestgen/collector` | Package AST | `TestSuiteSpecSet` — suites with attached methods |
| **Reduce** | `gotestast` | `TestSuiteSpecSet` | Effective set + skipped suites/cases (focus/exclude applied) |
| **Render** | `gotestgen/renderer` | `SpecOutcome` | Formatted Go source bytes |

Collection is a two-pass AST traversal:
1. `DetermineTestSuite`: Find type declarations matching the suite name pattern
2. `DetermineTestSuiteHarness`: Attach methods to their parent suites by matching receiver types

### Type Resolution for Generic Aliases

For regular suites, `pkg.TypesInfo.TypeOf(ts.Type)` returns `*types.Struct` directly.

For generic aliases like `type StringTestSuite = GenericTestSuite[string]`:
- `TypeOf(ts.Type)` returns `*types.Named` (not `*types.Struct`)
- The struct is obtained via `named.Underlying().(*types.Struct)`
- The underlying type name (`GenericTestSuite`) is stored for method matching

Method matching compares the receiver type name against both the suite's own name and the underlying type name of the alias.

---

## Testing Strategy

| Level | Tests | Approach |
|-------|-------|----------|
| **Unit** | `gotestast/spec_test.go`, `args_test.go`, `assert/base_test.go` | Table-driven tests for regex matching, arg parsing, assertions |
| **Generator** | `gotestgen/generator_test.go` | Golden-file comparison of generated code per example package |
| **E2E** | `e2e/cli_pmain_test.go` | Copy module to temp dir → run CLI as subprocess → compare output against golden files → verify cleanup |

### E2E Isolation

E2E tests use copy-on-write isolation:
1. Clone the entire module into `t.TempDir()`
2. Exclude `.git` and `go.work` from the copy
3. Activate dormant test files (`.go.test` → `.go`)
4. Create a temporary `go.work` with appropriate `use` and `replace` directives
5. Run the CLI as a subprocess
6. Assert output against golden files (with timestamp normalization)
7. Verify generated files are cleaned up after execution

### Golden File Conventions

- Located in `examples/<name>/testdata/` and `e2e/testdata/`
- Generator goldens: `gotestgen_ptest.golden`, `gotestgen_pxtest.golden`
- E2E output goldens: `<name>_output.txt`
- Timestamps replaced with `<TIMESTAMP>` placeholder
- Must begin with the `// Code generated` header (validated by test)

### Example Packages

| Package | Purpose |
|---------|---------|
| `examples/stdlib` | Mixed package: traditional tests + suite-based tests coexisting |
| `examples/simple_suite` | Basic lifecycle hooks (BeforeAll/AfterAll/BeforeEach/AfterEach) with intentional test failure |
| `examples/focus_exclude` | Focus (`F_`) and exclude (`X_`) semantics for suites and test cases |
| `examples/parallel_suite` | Suite-level `t.Parallel()` + per-case `TestParallel*` with WaitGroup |
| `examples/generic_suite` | Generic type definition with instantiated type aliases |

---

## Known Limitations

1. **Generic aliases in `pxtest`**: Go does not allow defining methods on aliases of types from other packages. Generic suite aliases only work in same-package tests.

2. **Assertion stubs**: `Contains` (string case), `ContainsAll`, and `ContainsAny` are incomplete.

3. **`go.work` required for cross-module tests**: The generator golden tests require `go.work` to be set up (`go work init . && go work use ./examples`). Tests skip gracefully when absent.

4. **No incremental generation**: The tool regenerates all suite files on every run. There is no staleness detection.

5. **No custom test binary flags**: The package pattern heuristic cannot distinguish flag values from package patterns in all cases (e.g., `-run TestFoo` makes `TestFoo` a candidate). This is harmless — `packages.Load("TestFoo")` returns zero packages — but it's imprecise.
