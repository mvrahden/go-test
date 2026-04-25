# `testsuite` Branch — Feature Analysis

## Vision

The `testsuite` branch reimagines how the `go-test` framework operates. Instead of being a library-only solution, it introduces a **CLI tool (`cmd/testsuite`)** that wraps `go test` with a code-generation pipeline. Users write xUnit-style test suites as plain Go structs, and the CLI transparently generates the boilerplate `func Test*(t *testing.T)` entry points that Go's test runner requires — then cleans them up after the run.

---

## Architecture Overview

```
User invokes:  go run .../cmd/testsuite ./... -v
                         │
                         ▼
              ┌─── Parse CLI args ───┐
              │  (cmd/testsuite/args) │
              └──────────┬───────────┘
                         │  ExecConfig
                         ▼
              ┌─── Execution Engine ──┐
              │  (cmd/testsuite/exec) │
              │                       │
              │  Phase 1: Generate    │──▶ AST-scan packages, emit ƒƒ_psuite_test.go
              │  Phase 2: Run         │──▶ exec `go test` with original args
              │  Phase 3: Cleanup     │──▶ delete generated files
              └───────────────────────┘
```

---

## Feature Breakdown

### 1. CLI Test Runner (`cmd/testsuite`)

| Aspect          | Detail                                                                                                                                                                                          |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Entry point** | `cmd/testsuite/cli.go` — minimal `main()`: parse → run → exit                                                                                                                                   |
| **Arg parsing** | Custom parser (`args.go`) that separates internal flags (`-ƒƒ.*`) from `go test` flags. Identifies package targets from positional args without interfering with `go test`'s own flag semantics |
| **Execution**   | Three-phase pipeline (`exec.go`) with fan-out concurrency (bounded worker pool, default 8) for generation and cleanup. Test execution is a single `go test` invocation                          |
| **Debug mode**  | `-ƒƒ.internal.debug` flag skips cleanup, leaving generated files for inspection                                                                                                                 |

### 2. Code Generation Pipeline (`internal/gotestgen`)

The generator converts user-authored suite structs into standard Go test functions:

| Stage       | Package                   | Purpose                                                                                                        |
| ----------- | ------------------------- | -------------------------------------------------------------------------------------------------------------- |
| **Load**    | `gotestgen/load_cache.go` | Thread-safe cached `packages.Load` (syntax, types, module info)                                                |
| **Collect** | `gotestgen/collector.go`  | Two-pass AST traversal: (1) find `*TestSuite` type declarations, (2) attach methods to their parent suites     |
| **Analyze** | `gotestast/spec.go`       | Regex-based classification of struct names and methods into lifecycle hooks, test cases, focus/exclude markers |
| **Reduce**  | `gotestast/spec.go`       | Apply focus (`F_`) / exclude (`X_`) semantics to produce the effective test set                                |
| **Render**  | `gotestgen/renderer.go`   | Go `text/template` rendering with `go/format` post-processing                                                  |

**Generated output per package directory:**
- `ƒƒ_psuite_test.go` — for same-package (white-box) test suites
- `ƒƒ_pxsuite_test.go` — for external-package (black-box) test suites

The Unicode `ƒƒ` prefix is deliberate — it virtually eliminates naming collisions with user code and is auto-detected by the cleanup phase.

### 3. Test Suite Lifecycle Model

Users define suites as structs with pointer-receiver methods:

```go
type MyTestSuite struct {
    sut *MyService  // shared fixture
}

// All lifecycle hooks are optional:
func (s *MyTestSuite) BeforeAll(t *gotest.T)  {}  // once before suite
func (s *MyTestSuite) AfterAll(t *gotest.T)   {}  // once after suite
func (s *MyTestSuite) BeforeEach(t *gotest.T) {}  // before each test
func (s *MyTestSuite) AfterEach(t *gotest.T)  {}  // after each test

// Any Test* method becomes a test case:
func (s *MyTestSuite) TestSomething(t *gotest.T) { ... }
```

**Generated wrapper pattern:** For each suite, the generator emits a `ƒƒ_GOTEST_*` wrapper struct that embeds the user's suite and provides explicit lifecycle delegation. Unimplemented hooks become no-ops. `AfterAll` is registered via `t.Cleanup` (LIFO) to guarantee execution even on panics.

### 4. Focus & Exclude Semantics

Inspired by Jest/Jasmine, naming conventions control which suites and test cases run:

| Prefix   | Effect                                                        | Scope              |
| -------- | ------------------------------------------------------------- | ------------------ |
| `F_`     | **Focus** — only focused items run; all unfocused are skipped | Suite or test case |
| `X_`     | **Exclude** — always skipped, even if focused                 | Suite or test case |
| *(none)* | Normal — runs unless something else is focused                | Suite or test case |

Exclusion always wins over focus.

### 5. Parallel Test Support

| Convention                              | Meaning                                                            |
| --------------------------------------- | ------------------------------------------------------------------ |
| Struct name ends in `TestSuiteParallel` | Suite-level `t.Parallel()`                                         |
| Method name starts with `TestParallel`  | Test-case-level `it.Parallel()` with `sync.WaitGroup` coordination |
| Method name ends in `Async`             | Async test case receiving a `done func()` callback                 |

The generated code uses a `sync.WaitGroup` to ensure `AfterAll` waits for all parallel subtests to complete before running.

### 6. Fluent Assertion API (`pkg/gotest/internal/assert`)

The `gotest.T` wrapper exposes `t.Assert(value)` returning a fluent `BaseAssertionContext`:

| Method                                    | Description                                             | Status                            |
| ----------------------------------------- | ------------------------------------------------------- | --------------------------------- |
| `IsTrue()` / `IsFalse()`                  | Boolean assertions                                      | Complete                          |
| `IsEqualTo(v)`                            | Deep equality via `reflect.DeepEqual`                   | Complete                          |
| `IsZero()`                                | Zero-value check via `reflect.Value.IsZero()`           | Complete                          |
| `IsEmpty()`                               | Length == 0 for strings, maps, slices, arrays, channels | Complete                          |
| `HasLength(n)`                            | Exact length check                                      | Complete                          |
| `HasCapacity(n)`                          | Exact capacity check for slices, arrays, channels       | Complete                          |
| `Contains(v)`                             | Element containment                                     | Partial (string case is a no-op)  |
| `ContainsAll(v...)` / `ContainsAny(v...)` | Multi-element containment                               | Stub (`panic("not implemented")`) |

Assertion failures route through `t.Fatalf`, immediately failing the current subtest. A `{{FMT}}` token system produces human-readable error messages that print type names instead of raw pointer addresses.

### 7. BDD-Style Nesting

`t.It(description, func(it *gotest.T))` provides Jasmine/RSpec-like nested subtests on top of `t.Run`, enabling:

```go
func (s *MyTestSuite) TestSomething(t *gotest.T) {
    t.It("succeeds when input is valid", func(it *gotest.T) {
        it.Assert(result).IsEqualTo(expected)
    })
}
```

### 8. Stdlib Co-existence

The framework explicitly supports **mixed test packages** — traditional `func Test*(t *testing.T)` functions and suite-based tests coexist in the same package. The `examples/stdlib` package demonstrates this. No migration pressure.

---

## Testing Strategy

| Level                     | Location                                                             | Approach                                                                                                                      |
| ------------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| **Unit**                  | `gotestast/spec_test.go`, `args_unit_test.go`, `assert/base_test.go` | Table-driven tests for regex matching, arg parsing, assertions                                                                |
| **Generator integration** | `gotestgen/generator_test.go`, `testgen_e2e_test.go`                 | Golden-file comparison of generated code                                                                                      |
| **Full E2E**              | `e2e/cli_pmain_test.go`, `pkg/gotest/main_test.go`                   | Copy module to temp dir → run CLI as subprocess → compare output against golden files → verify generated files are cleaned up |

The E2E tests use a **copy-on-write isolation** pattern: the entire module is cloned into a temp directory with `go.work` workspace setup, ensuring zero side effects. Dormant test files (`.go.test` extension) are activated only inside the temp copy.

---

## Notable Design Decisions

1. **Code generation over reflection** — Unlike testify/suite which uses `reflect` at runtime, this tool generates concrete Go code at build time, producing standard `go test` output with zero reflection overhead.
2. **Transparent `go test` wrapper** — All flags pass through verbatim. The CLI only intercepts package names and its own `ƒƒ`-namespaced flags.
3. **Ephemeral generated files** — Generated files exist only during the test run and are cleaned up automatically, keeping the working tree clean.
4. **Dual package mode** — Full support for both same-package (`ptest`) and external-package (`pxtest`) test suites, respecting Go's two-package-per-directory convention.
5. **Unicode namespacing** — The `ƒƒ` prefix for generated files and internal types makes collisions with user code virtually impossible.

---

## Incomplete / In-Progress Items

- `Contains` assertion for strings is a no-op (empty case body)
- `ContainsAll` / `ContainsAny` are stubs that panic
- `SortStable` in `internal/x/slices` calls `sort.Slice` instead of `sort.SliceStable`
- `StdlibRunTests` in `gotestrunner/stdlib.go` appears to double-append args
