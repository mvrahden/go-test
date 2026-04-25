# go-test Superspec

## Vision

Go tests that write themselves, organize themselves, and explain themselves.

Go's testing framework is deliberately minimal — and that's a strength. But the gap between `func TestX(t *testing.T)` and a well-organized, maintainable test suite is filled entirely by manual boilerplate. `go-test` closes that gap through code generation: you write structs, name them well, and the tool handles the rest. No runtime dependencies. No reflection. No lock-in. Just standard Go tests with lifecycle management, structured organization, and development-time ergonomics that make testing feel effortless.

The north star: **a Go developer should be able to `go install` this tool and immediately write better-organized tests without learning a framework.** The naming conventions are the API. The generated code is the implementation. The output is `go test` output. Nothing is hidden.

---

## Design Philosophy

These principles are ranked. When they conflict, higher-ranked principles win.

### 1. Standard Go output, always

Every test the tool generates is a `func Test*(t *testing.T)`. Every line of output is standard `go test` output. Every CI system, IDE, coverage tool, and profiler works unchanged. If a feature would require custom output parsing or a custom test runner, we don't build it.

### 2. The naming IS the API

No config files. No struct tags. No annotations. No registration calls. A developer reads the naming conventions once and never opens documentation again. If a behavior can't be expressed through a naming convention that a Go developer would guess correctly on first try, we reconsider the behavior.

### 3. Zero runtime cost

The tool generates code at build time and cleans it up after. At test execution time, there is no `go-test` code in the call stack (except the thin `gotest.T` wrapper). No reflection, no interface dispatch, no type assertions. The generated code is what a careful developer would write by hand — `defer`, `t.Run`, `t.Cleanup`, `sync.WaitGroup`.

### 4. Invisible until needed

A developer who has never heard of `go-test` can read a test suite struct and understand what it does. The `BeforeEach`, `TestSomething`, `AfterAll` method names are self-documenting. A developer who runs `go test` directly (without the CLI) gets a compilation error for the missing generated file — not silent wrong behavior.

### 5. Adopt incrementally, eject freely

Existing `func Test*` tests coexist with suites in the same package. Removing the tool means deleting suite structs and writing the equivalent `func Test*` functions — the generated code shows exactly what to write. There is no migration, no vendor lock-in, no "all or nothing."

---

## Conceptual Model

The project's identity rests on a single insight: **test suites are behavioral specifications.** Every level of the test hierarchy maps to a specification concept:

```
struct  = Subject     "UserService"
method  = Capability  "Create"
When()  = Context     "when email is valid"
It()    = Behavior    "creates the user"
Each()  = Variants    "standard format", "missing @", "empty string"
```

The naming conventions at the struct/method level and the string descriptions at the `It`/`When` level together form a complete behavioral specification. The tool generates the bridge code (lifecycle, parallel coordination, focus/exclude) and can render the full specification in human-readable form.

This framing drives every feature decision: if a feature doesn't help tests **write themselves**, **organize themselves**, or **explain themselves**, it doesn't belong.

---

## CLI Interface

The `ƒƒ` Unicode prefix is kept for generated filenames and internal types (machine-only, collision-resistant), but removed entirely from the CLI interface. The CLI uses standard subcommands and flags — zero special characters:

```
gotest [subcommand] [packages...] [go-test-flags...] [--gotest-flags...]
```

### Subcommands (operational modes)

| Command | Effect |
|---------|--------|
| *(default)* | Generate suites, run `go test`, cleanup |
| `watch` | Re-run on file changes |
| `clean` | Remove orphaned generated files |
| `generate` | Generate suite files without running tests |
| `scaffold` | Generate test suite skeleton from a Go type |
| `migrate` | Convert testify/suite tests to go-test suites |
| `spec` | Run tests and render behavioral specification |
| `coverage` | Report semantic test coverage |
| `version` | Print version information |
| `help` | Show help |

### Flags (options, apply in any mode)

| Flag | Effect |
|------|--------|
| `--debug` | Keep generated files after run |
| `--ci` | Fail if `F_` focus prefixes exist |
| `--spec` | Append spec summary after normal output |
| `--update-snapshots` | Regenerate snapshot files |
| `--format=<fmt>` | Output format for `spec` / `coverage` (terminal, md) |
| `--output=<path>` | Write formatted output to file |
| `--min=<pct>` | Minimum semantic coverage threshold for `coverage` |

### Disambiguation

The first positional arg is checked against the known subcommand set. If it matches, it's consumed. Otherwise, it's a package pattern. `gotest ./watch` builds the `watch` package; `gotest watch` enters watch mode. This is the same pattern as the `go` tool itself.

### Examples

```bash
gotest ./... -v -race                    # run tests (default mode)
gotest watch ./... -v                    # watch mode with verbose output
gotest clean ./...                       # remove orphaned generated files
gotest generate ./...                    # generate only, no test execution
gotest scaffold ./pkg/user.UserService   # generate suite skeleton
gotest spec ./...                        # run tests, show behavioral spec
gotest spec ./... --format=md --output=docs/spec.md
gotest coverage ./pkg/user --min=90      # semantic coverage check
gotest ./... --ci -v -race               # CI mode (fail on F_ prefixes)
gotest ./... --debug                     # keep generated files for inspection
```

### Migration from `-ƒƒ.*` flags

| Old | New | Notes |
|-----|-----|-------|
| `-ƒƒ.clean` | `gotest clean` | Subcommand |
| `-ƒƒ.internal.debug` | `--debug` | Flag |
| `-ƒƒ.ci` | `--ci` | Flag |
| `-ƒƒ.watch` | `gotest watch` | Subcommand |
| `-ƒƒ.generate` | `gotest generate` | Subcommand |
| `-ƒƒ.version` | `gotest version` | Subcommand |
| `-ƒƒ.update-snapshots` | `--update-snapshots` | Flag |

The old `-ƒƒ.*` forms are recognized as deprecated aliases during a transition period, then removed.

---

## Tier 1 — Ship It

*Production-ready core. What's needed before anyone outside the team uses this.*

### 1.1 Focus Guard: `--ci`

**Why:** Without `--ci`, the `F_` prefix is a footgun — a developer forgets to remove it, commits, and CI runs only one suite while reporting "all tests pass." This is a silent, catastrophic failure. With `--ci`, `F_` becomes a **safe, default development workflow:**

1. Prefix with `F_` — instant focused feedback during development
2. Remove `F_` — full suite confirmation before commit
3. `--ci` in CI — safety net catches any `F_` that slipped through
4. **Zero risk, maximum speed**

**What:** A static analysis scan before generation — no test execution needed:

```
$ gotest --ci ./...
FAIL: focus prefix detected — remove F_ before merging:
  pkg/user/user_test.go:12    type F_UserServiceTestSuite
  pkg/payment/pay_test.go:28  func (s *PaymentTestSuite) F_TestCharge
```

5 lines of code to implement. Prevents hours of debugging.

### 1.2 Assertion Library: Zero Dependencies, Full Type Safety

**Why:** The current fluent API (`t.Assert(v).IsEqualTo(w)`) is expressive but untyped — comparing a `string` to an `int` compiles. `ContainsAll`/`ContainsAny` panic. Meanwhile, `origin/main` has a generic typed assertion layer with excellent API design, but it vendors 2,300 lines of testify internals and pulls `go-spew` + `go-difflib` into `go.mod` — leaking transitive dependencies to every consumer of the module.

**Decision: Adopt the API design from `origin/main`. Rewrite the core from scratch.**

The generic typed signatures from `origin/main`'s `require` layer are the right API. The underlying assertion logic is reimplemented using Go stdlib only — no vendored testify, no `go-spew`, no `go-difflib`. ~300 lines of core logic replace ~3,000 lines of vendored code. Zero external dependencies.

**Architecture:**

```
pkg/gotest/                              ← public API (functional + fluent)
  internal/
    assert/                              ← core implementation (~300 lines, pure stdlib)
      equal.go                           ← reflect.DeepEqual + diff rendering
      compare.go                         ← cmp.Compare for ordered types
      format.go                          ← value formatting + minimal unified diff
```

The implementation uses only stdlib:

| Concern | testify/main approach | Reimplemented with |
|---------|----------------------|--------------------|
| Deep equality | `objectsAreEqual` (reflect.DeepEqual + wrappers) | `reflect.DeepEqual` directly |
| Ordered comparison | 500-line type switch (`assertion_compare.go`) | `cmp.Compare[T]` (Go 1.21) |
| Value formatting | `go-spew` (abandoned since 2018) | `fmt.Sprintf("%#v", v)` |
| Diff output | `go-difflib` | Minimal unified diff (~50 lines) |
| JSON comparison | `encoding/json` | `encoding/json` (same) |
| HTTP assertions | 165 lines | Not included (out of scope) |
| YAML comparison | vendored yaml shim | Not included (out of scope) |

**Functional API** — type-safe, one import, identical names to testify's `require`:

```go
import "github.com/mvrahden/go-test/pkg/gotest"

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
gotest.ErrorAs[E error](t, err error, msgAndArgs ...any) E     // returns matched error
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

// Serialization — accepts string, []byte, json.RawMessage, io.Reader, or any marshalable value
gotest.JSONEq(t, expected, actual any, msgAndArgs ...any)

// Time
gotest.TimeWithin(t, expected, actual time.Time, tolerance time.Duration, msgAndArgs ...any)
gotest.TimeIsNow(t, ts time.Time, tolerance time.Duration, msgAndArgs ...any)

// Panic
gotest.Panics(t, f func(), msgAndArgs ...any) any

// Async — simple boolean polling (see Tier 3 for rich assertion polling via t.Eventually)
gotest.Eventually(t, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...any)
gotest.Consistently(t, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...any)

// Unwrap — panics on failure (no t parameter, so Go's multi-return expansion works)
gotest.Must[T any](val T, ok any) T
```

All functions accept any type implementing `testingT` (`Helper()` + `Errorf()` + `FailNow()`) — this includes both `*gotest.T` and `*testing.T`, so the assertions work in suites and in standalone `func Test*` functions alike.

**Design notes:**

- **`Eventually` vs `Consistently`:** `Eventually` polls until the condition returns `true` or the timeout expires. `Consistently` asserts the condition stays `true` for the entire duration — it fails on the first `false`. These are the simple boolean forms; for rich assertion blocks with detailed failure messages on timeout, use `t.Eventually` / `t.Consistently` (Tier 3 — methods on `*gotest.T` that accept `func(poll *gotest.T)` instead of `func() bool`).
- **`Must` panics** instead of calling `t.FailNow()` because it takes no `t` parameter — this is what enables `gotest.Must(fn())` with Go's multi-return expansion. The test runner catches the panic and reports it as a test failure with a full stack trace. Don't use `Must` when you need a custom failure message — use `gotest.NoError` or `gotest.True` instead.

**Fluent API** — discoverable via autocomplete, delegates to the functional layer:

```go
t.Assert(result).Equal(expected)       // same as gotest.Equal(t, expected, result)
t.Assert(items).HasLength(3)           // same as gotest.Len(t, items, 3)
t.Assert(items).Contains("needle")     // same as gotest.Contains(t, items, "needle")
t.Assert(err).IsZero()                 // same as gotest.Zero(t, err)
t.Assert(ok).IsTrue()                  // same as gotest.True(t, ok)
t.Assert(err).NoError()                // same as gotest.NoError(t, err)
t.Assert(count).IsNotZero()            // same as gotest.NotZero(t, count)
```

The fluent API accepts `any`, trading compile-time type safety for discoverability — `gotest.Equal(t, 42, "hello")` is a compile error, but `t.Assert(42).Equal("hello")` compiles and fails at runtime. Use fluent for quick exploration and autocomplete-driven discovery; use functional for production test suites where type safety matters.

**Diff output for equality failures:**

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

All functions call `t.Helper()` so failures report the caller's file:line, not the assertion library's internals. The diff renderer uses `%#v` for Go-syntax formatting. Pointer addresses are never shown. For strings, a line-by-line unified diff. For structs/maps/slices, field-level `-`/`+` markers.

**Migration from testify:** Function names are identical — `Equal`, `NoError`, `ErrorIs`, `Contains`, `Len`, etc. Migration is an import path change plus a receiver rename:

```diff
- import "github.com/stretchr/testify/require"
+ import "github.com/mvrahden/go-test/pkg/gotest"

- require.Equal(t, expected, actual)
+ gotest.Equal(t, expected, actual)
```

**Dogfooding:** The project's own tests use this assertion library instead of `stretchr/testify`, eliminating the last external test dependency.

**Deliberately excluded:**

- HTTP-specific assertions (out of scope for a test suite framework)
- YAML comparison (niche, adds dependency or complexity)
- `go-spew` pretty-printing (`%#v` is sufficient; `go-spew` is unmaintained since 2018)
- The 2,272-line vendored testify `assertions.go` (reimplemented in ~300 lines)
- `go-difflib` (replaced by ~50-line inline diff renderer)

### 1.3 CLI Distribution

**Why:** `go run github.com/mvrahden/go-test/cmd/testsuite ./...` is 40 characters of incantation. Developers need `gotest ./...`.

**What:**

- **Binary name:** `gotest`
- **Install:** `go install github.com/mvrahden/go-test/cmd/gotest@latest`
- **Invocation:** `gotest ./... -v -race -count=1` — identical argument order to `go test`
- **Shell alias guidance:** `alias got='gotest'` for the truly lazy
- **Version:** `gotest version` prints version, Go version, OS/arch
- **goreleaser** for cross-platform binaries and GitHub releases

### 1.4 README and Examples

**Why:** No README = no adoption. The README is the product for open-source tools.

**What:**

```
# gotest

Go tests that write themselves, organize themselves, and explain themselves.

## Install
  go install github.com/mvrahden/go-test/cmd/gotest@latest

## 30-Second Example
  [show a complete suite → generated output → test output]

## Features
  [lifecycle hooks, focus/exclude, parallel, generics — one paragraph each]

## How It Works
  [code generation diagram, 3 sentences]

## Reference
  [link to spec.md for full details]
```

No walls of text. Code-heavy. A developer should go from "what is this" to "running their first suite" in under 2 minutes.

---

## Tier 2 — Adopt It

*Zero-friction onboarding. Meet developers where they are.*

### 2.1 Auto-Scaffolding: `gotest scaffold`

**Why:** The #1 adoption barrier is the cold start. A developer stares at a new type and spends 20-40 minutes writing test boilerplate before the first real assertion. The tool already has `packages.Load` with full type information — running the pipeline in reverse (from production code to test skeletons) is the natural dual of what it already does.

**What:**

```
$ gotest scaffold ./pkg/user.UserService

Generated: pkg/user/user_service_suite_ptest_test.go
```

```go
type UserServiceTestSuite struct {
    sut *UserService
}

func (s *UserServiceTestSuite) BeforeEach(t *gotest.T) {
    s.sut = NewUserService( /* TODO: dependencies */ )
}

func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.It("succeeds with valid input", func(it *gotest.T) {
        // TODO: test UserService.Create(ctx context.Context, user *User) error
    })

    t.It("returns error with invalid input", func(it *gotest.T) {
        // TODO
    })
}

func (s *UserServiceTestSuite) TestGetByID(t *gotest.T) {
    t.It("returns the user", func(it *gotest.T) {
        // TODO: test UserService.GetByID(ctx context.Context, id string) (*User, error)
    })

    t.It("returns error when not found", func(it *gotest.T) {
        // TODO
    })
}

func (s *UserServiceTestSuite) TestUpdate(t *gotest.T) {
    t.It("updates the user", func(it *gotest.T) {
        // TODO: test UserService.Update(ctx context.Context, user *User) error
    })
}

func (s *UserServiceTestSuite) TestDelete(t *gotest.T) {
    t.It("deletes the user", func(it *gotest.T) {
        // TODO: test UserService.Delete(ctx context.Context, id string) error
    })
}
```

**How it works:** The scaffolder uses `packages.Load` to introspect the target type. For each exported method, it generates a `Test*` method with `t.It()` blocks. The method signature is included as a comment. Return types inform the `It` block stubs — methods returning `error` get a happy-path and an error-case stub.

**Interface scaffolding** generates a generic contract suite:

```
$ gotest scaffold --contract io.ReadCloser
```

```go
type ReadCloserContractTestSuite[T io.ReadCloser] struct {
    factory func() T
    sut     T
}

func (s *ReadCloserContractTestSuite[T]) BeforeEach(t *gotest.T) {
    s.sut = s.factory()
}

func (s *ReadCloserContractTestSuite[T]) TestRead(t *gotest.T) {
    t.It("reads bytes into buffer", func(it *gotest.T) {
        // TODO: test Read(p []byte) (n int, err error)
    })
}

func (s *ReadCloserContractTestSuite[T]) TestClose(t *gotest.T) {
    t.It("releases resources", func(it *gotest.T) {
        // TODO: test Close() error
    })
}

// Instantiate for your implementation:
// type MyReaderTestSuite = ReadCloserContractTestSuite[*MyReader]
```

**Why this is transformative:** The 20-40 minute cold start becomes 5 seconds. The naming conventions are learned by example, not documentation. The developer's first interaction with go-test is `scaffold` producing working code — not reading a README.

### 2.2 Migration Path: `gotest migrate`

**Why:** Teams with hundreds of testify/suite tests won't switch if it means rewriting. A migration tool removes this barrier entirely.

**What:**

```
$ gotest migrate ./...

Migrated 12 suites across 8 packages:
  pkg/user/user_test.go:   suite.Run(t, new(UserSuite))  → UserTestSuite
  pkg/order/order_test.go: suite.Run(t, new(OrderSuite)) → OrderTestSuite
  ...
```

The migration:
1. Finds `suite.Run(t, &MySuite{})` / `suite.Run(t, new(MySuite))` patterns
2. Renames the suite struct to end in `TestSuite` (if it doesn't already)
3. Renames `SetupSuite` → `BeforeAll`, `TearDownSuite` → `AfterAll`, `SetupTest` → `BeforeEach`, `TearDownTest` → `AfterEach`
4. Changes `s.T()` → `t.T()` and `s.Require().Equal(a, b)` → `gotest.Equal(t, a, b)`
5. Removes the `func Test*(t *testing.T) { suite.Run(...) }` boilerplate
6. Removes the `testify/suite` import

This is a one-time AST transformation. It handles the 90% case and leaves `// TODO: manual review` comments for edge cases.

### 2.3 BDD Vocabulary: `t.When()`

**Why:** `t.It()` already exists for specifying behaviors. But test cases need context grouping — "when the user is authenticated", "when the database is empty." Nesting `It` inside `It` reads poorly (`it.It(...)`). A `When` primitive provides semantic clarity and makes the spec output readable.

**What:**

```go
func (s *UserServiceTestSuite) TestCreate(t *gotest.T) {
    t.When("email is valid", func(w *gotest.T) {
        w.It("creates the user", func(it *gotest.T) {
            err := s.svc.Create(ctx, validUser)
            gotest.NoError(it, err)
        })
        w.It("sends a welcome email", func(it *gotest.T) {
            gotest.Eventually(it, func() bool {
                return s.mailer.HasMessage(validUser.Email)
            }, 2*time.Second, 50*time.Millisecond)
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

**Implementation:** One method on `gotest.T`:

```go
func (t *T) When(description string, fn func(w *T)) {
    t.t.Run(description, func(tt *testing.T) {
        fn(NewT(tt))
    })
}
```

Identical to `t.It()` under the hood — both map to `t.T().Run()`. The distinction is purely semantic: `When` provides context (grouping node), `It` specifies an expectation (leaf node). The spec renderer uses this to produce structured output.

---

## Tier 3 — Love It

*Daily workflow features. What makes developers choose this over everything else.*

### 3.1 Behavior Specification: `gotest spec`

**Why:** Test suites ARE behavioral specifications — but only if you can read them as such. Raw `go test -v` output is optimized for machines. The spec view presents the same information as a readable, structured behavioral document.

**What:**

```
$ gotest spec ./pkg/user -v

UserService
  Create
    when email is valid
      ✓ creates the user (8ms)
      ✓ sends a welcome email (120ms)
    when email already exists
      ✓ returns ErrDuplicate (<1ms)
  GetByID
    ✓ returns the user (3ms)
    ✗ returns error when not found (2ms)
        ErrorIs failed:
          expected: ErrNotFound
          actual:   <nil>
          location: user_service_test.go:72
  Delete
    ✓ soft-deletes the user (5ms)
    ~ hard-deletes after 30 days — SKIPPED

F_PaymentService — FOCUSED
  Charge
    ✓ processes valid amount (45ms)

3 suites, 8 behaviors: 6 passed, 1 failed, 1 skipped
```

**How it works:**

1. Run `go test -json ./...` internally
2. Parse the JSON event stream — every `t.Run` (including `t.It` and `t.When`) produces events with `/`-separated paths
3. Reconstruct the hierarchy: first segment = suite (`TestUserServiceTestSuite`), second = method (`TestCreate`), rest = `When`/`It` blocks
4. Strip Go naming conventions: `TestUserServiceTestSuite` → `UserService`, `TestCreate` → `Create`
5. Render as a behavioral specification tree

**The full test hierarchy:**

```
Package
  Suite (struct name, strip "TestSuite" suffix)
    Test Case (method name, strip "Test" prefix)
      When block (description string — context)
        It block (description string — expectation)
          Each entries (Desc field or index — variants)
```

**Output formats:**

```bash
# Terminal: color-coded tree (default for spec subcommand)
gotest spec ./...

# Markdown: living specification document
gotest spec ./... --format=md --output=docs/behavior-spec.md

# Append spec summary after normal go test output
gotest ./... -v --spec
```

**Markdown output:**

```markdown
# Behavior Specification

Generated from test suites on 2026-04-25. 47 behaviors: 45 passed, 1 failed, 1 skipped.

## UserService

### Create

| Behavior | Status | Duration |
|----------|--------|----------|
| **when email is valid** | | |
| creates the user | PASS | 8ms |
| sends a welcome email | PASS | 120ms |
| **when email already exists** | | |
| returns ErrDuplicate | PASS | <1ms |

### Delete

| Behavior | Status | Duration |
|----------|--------|----------|
| soft-deletes the user | PASS | 5ms |
| hard-deletes after 30 days | SKIP | — |
```

PRs can include a spec diff: "this PR adds 3 new behaviors to UserService and removes 1 from OrderService." Code reviewers see behavioral intent, not just line changes.

**Why this is NOT a "custom test reporter" (non-goal):** The spec view does not replace `go test` output. It post-processes `go test -json` — the same data that `gotestfmt` and `tparse` consume. The unique value is the structural mapping from suite hierarchy to behavioral specification, which only this tool can provide because it understands the suite→method→It/When model.

### 3.2 Watch Mode

**Why:** The tightest feedback loop wins. A developer iterating on a feature wants to save a file and see test results instantly, without switching windows to run a command.

**What:**

```bash
gotest watch ./... -v
gotest watch ./... --spec     # watch + spec view
```

Behavior:
1. Initial run: full generate → test → cleanup cycle
2. Watch filesystem for `.go` file changes (via `fsnotify`)
3. On change: re-run the generate → test → cleanup cycle for affected packages
4. Display: clear terminal, show results, show "watching..." prompt
5. On Ctrl+C: cleanup and exit

**Package-scoped re-runs:** When `pkg/foo/bar.go` changes, re-run `./pkg/foo/...` not the entire pattern. Use Go's package graph to determine the minimal affected set.

**Focus integration:** During watch mode, `F_`-prefixed suites create an extremely tight loop — only the focused tests re-run on every save:

1. `gotest watch ./...` in a terminal
2. Prefix the suite you're working on with `F_`
3. Save → only that suite runs → instant feedback
4. Remove the `F_` prefix → all suites run → confirm nothing broke
5. Commit (with `--ci` in CI to catch any remaining `F_` prefix)

**Debouncing:** Multiple rapid file saves (e.g., editor auto-save, `goimports`) trigger a single re-run after 200ms of quiet.

### 3.3 Data-Driven Testing: `t.Each()`

**Why:** Table-driven tests are Go's most common pattern but require verbose boilerplate: define a struct, build a slice, range over it, call `t.Run`. `t.Each` reduces this to two lines.

**What:**

```go
func (s *Suite) TestValidation(t *gotest.T) {
    t.It("validates email format", func(it *gotest.T) {
        it.Each([]struct {
            Desc  string
            Email string
            Valid bool
        }{
            {"accepts standard format", "user@example.com", true},
            {"rejects missing @", "userexample.com", false},
            {"rejects empty string", "", false},
        }, func(tt *gotest.T, tc struct{ Desc, Email string; Valid bool }) {
            gotest.Equal(tt, s.validator.IsValid(tc.Email), tc.Valid)
        })
    })
}
```

Each table entry becomes a `t.Run` subtest. If the table struct has a `Desc` or `Name` field, it's used as the subtest name. Otherwise, entries are indexed.

**Spec output:**

```
UserService
  Validation
    validates email format
      ✓ accepts standard format
      ✗ rejects missing @
      ✓ rejects empty string
```

This is a runtime feature on `gotest.T` — no code generation changes needed.

### 3.4 Async Assertions: `t.Eventually()` / `t.Consistently()`

**Why:** The `time.Sleep` anti-pattern is the #1 source of flaky integration tests. Every team writes their own polling helper. This should be a primitive.

**Two tiers of async assertion:**

The functional API (Tier 1) provides simple boolean polling:

```go
// Simple: "wait until ready"
gotest.Eventually(t, func() bool { return s.svc.IsReady() }, 5*time.Second, 100*time.Millisecond)
```

When this fails, the message is "Condition never satisfied" — adequate for one-liners, but useless for multi-step checks. The T method (this tier) provides **rich assertion polling** — full assertion context inside the callback, with detailed failure messages on timeout:

```go
func (s *Suite) TestAsyncProcessing(t *gotest.T) {
    s.svc.TriggerAsync()

    t.Eventually(5*time.Second, 100*time.Millisecond, func(poll *gotest.T) {
        result, err := s.store.Get("key")
        gotest.NoError(poll, err)
        gotest.Equal(poll, result.Status, "completed")
    })
}
```

**Signatures:**

```go
// Methods on *gotest.T — rich assertion callback
func (t *T) Eventually(waitFor, tick time.Duration, fn func(poll *T))
func (t *T) Consistently(waitFor, tick time.Duration, fn func(poll *T))
```

Configuration (timeout, tick) comes first; the closure goes last — so multi-line bodies close naturally without trailing arguments buried after the `}`.

**Failure suppression mechanism:** The `poll *gotest.T` parameter is a collecting wrapper — assertion failures during intermediate polls are captured but not propagated to the test. Only when the timeout expires does the method report the **last poll's assertion failures** as the test failure. This means:

```
Eventually failed after 5s (50 polls):
  last failure:
    Equal failed:
      expected: "completed"
      actual:   "pending"
      location: async_test.go:42
```

The developer sees exactly what condition wasn't met, not just "timed out."

**`Consistently`** is the dual — asserts the function passes on **every** poll for the entire duration. It detects flicker: a condition that's usually true but occasionally false.

```go
t.Consistently(2*time.Second, 100*time.Millisecond, func(poll *gotest.T) {
    gotest.Equal(poll, s.counter.Value(), 0) // must stay zero for 2 full seconds
})
```

On failure, `Consistently` reports which poll first failed and what the assertion said — "passed 15 times then failed on poll 16."

Both are runtime features on `gotest.T`. No code generation changes.

### 3.5 Snapshot Testing: `t.MatchSnapshot()`

**Why:** Golden-file testing is powerful but manual. Developers copy-paste expected output, and it drifts. Snapshot testing auto-manages expectations.

**What:**

```go
func (s *Suite) TestJSONOutput(t *gotest.T) {
    result := s.sut.ToJSON()
    t.MatchSnapshot(result)
}
```

Behavior:
- Snapshots stored in `testdata/__snapshots__/<TestName>.snap`
- First run with no snapshot: create it, pass the test, print "created snapshot"
- Subsequent runs: compare against snapshot, fail with diff on mismatch
- Update: `gotest ./... --update-snapshots` regenerates all snapshots

**Multi-snapshot per test:**

```go
t.MatchSnapshot(s.sut.ToJSON(), "json")
t.MatchSnapshot(s.sut.ToYAML(), "yaml")
```

---

## Tier 4 — Depend On It

*Organizational value. Why teams standardize on this tool.*

### 4.1 Semantic Test Coverage: `gotest coverage`

**Why:** Line coverage answers "which code ran?" Semantic coverage answers "which behaviors are verified?" The tool knows the public API surface of every type (via `packages.Load`) and which methods have corresponding test cases (via suite analysis). The gap between these two sets is the semantic coverage gap.

**What:**

```
$ gotest coverage ./pkg/user

UserService: 4/5 methods covered (80%)
  ✓ Create         — TestCreate (3 behaviors)
  ✓ GetByID        — TestGetByID (2 behaviors)
  ✓ Update         — TestUpdate (1 behavior)
  ✗ Delete         — no test case
  ✓ ListByOrg      — TestListByOrg (2 behaviors)

User: 2/3 exported methods covered (67%)
  ✓ Validate       — TestValidate (5 behaviors via Each)
  ✓ FullName       — TestFullName (1 behavior)
  ✗ MarshalJSON    — no test case

Overall: 6/8 methods covered (75%)
```

**This doesn't require running tests.** It's a static analysis step that compares the production API surface against the suite test method inventory. It can run in CI as a quality gate:

```bash
$ gotest coverage ./... --min=90
FAIL: pkg/user has 75% semantic coverage (minimum 90%)
  missing: UserService.Delete, User.MarshalJSON
```

**Behavior count:** When tests are actually run (`gotest coverage ./... -v`), the tool also counts `t.It()` blocks per method, giving a deeper view of behavioral coverage.

### 4.2 CI Integration

**Why:** The tool must work in CI without special configuration.

**What:**

- **GitHub Actions action:** `uses: mvrahden/setup-gotest@v1` installs the binary and caches it
- **Exit codes:** Match `go test` exit codes exactly (0 = pass, 1 = test failure, 2 = build error)
- **`--ci` flag:** Fails on `F_` prefixes (see Tier 1)
- **Spec report in CI:** `gotest spec ./... --format=md --output=spec.md` produces an artifact

```yaml
# .github/workflows/test.yml
- uses: mvrahden/setup-gotest@v1
- run: gotest --ci ./... -v -race
- run: gotest coverage ./... --min=80
- run: gotest spec ./... --format=md --output=behavior-spec.md
- uses: actions/upload-artifact@v4
  with:
    name: behavior-spec
    path: behavior-spec.md
```

### 4.3 Go Generate Integration

**Why:** Some teams prefer `go generate` workflows over custom CLI wrappers.

**What:**

```go
//go:generate gotest generate ./...
```

The `generate` subcommand runs only the generation step (no test execution, no cleanup). The developer then runs `go test` normally. Useful for:
- Teams that want generated files checked in (inspectable, diffable)
- Pre-commit hooks that validate generated files are up-to-date
- Build systems that separate generation from execution

### 4.4 Linter: `gotest-lint`

**Why:** Naming conventions are the API, but naming mistakes are silent. A method named `BeforAll` (typo) is just a regular method — the generator ignores it, and the developer's setup code never runs.

**What:**

A static analysis tool (standalone or `golangci-lint` plugin) that warns about:

- **Likely typos:** `BeforAll`, `AfterEeach`, `Testfoo` — methods on suite structs that are close to but don't match known patterns
- **Wrong receiver type:** Value receivers on suite methods
- **Missing lifecycle hook:** A suite with `BeforeAll` but no `AfterAll` — may leak resources (warning, not error)
- **Focused tests committed:** `F_` prefix in non-test-infrastructure code (overlaps with `--ci` but runs without the CLI)
- **Orphaned generated files:** `ƒƒ_*` files checked into version control

---

## Advanced Patterns

*Capabilities that emerge from the core model.*

### Nested Suites via Embedding

A suite that embeds another suite inherits its lifecycle hooks:

```go
type UserServiceTestSuite struct {
    db   *TestDB
    svc  *UserService
}

func (s *UserServiceTestSuite) BeforeAll(t *gotest.T) {
    s.db = NewTestDB()
    s.svc = NewUserService(s.db)
}

func (s *UserServiceTestSuite) AfterAll(t *gotest.T) {
    s.db.Close()
}

type UserCreateTestSuite struct {
    UserServiceTestSuite
    user *User
}

func (s *UserCreateTestSuite) BeforeEach(t *gotest.T) {
    s.user = &User{Name: "test"}
}

func (s *UserCreateTestSuite) TestCreatesUser(t *gotest.T) {
    t.It("persists the user", func(it *gotest.T) {
        err := s.svc.Create(s.user)
        gotest.NoError(it, err)
    })
}
```

**Generated lifecycle chain:**

```
UserServiceTestSuite.BeforeAll (parent)
└── UserCreateTestSuite
    ├── UserServiceTestSuite.BeforeEach (parent, if defined)
    │   └── UserCreateTestSuite.BeforeEach (child)
    │       └── TestCreatesUser
    │       └── UserCreateTestSuite.AfterEach (child, if defined)
    │   └── UserServiceTestSuite.AfterEach (parent, if defined)
    ├── [next test...]
UserServiceTestSuite.AfterAll (parent)
```

**Spec output:**

```
UserService
  Create
    ✓ persists the user (12ms)
    ✓ rejects duplicate (3ms)
  Update
    ...
```

The spec renderer understands the parent-child relationship and nests the child suite under the parent's name when the child's name extends the parent's name.

### Contract Testing via Generic Suites

Generic type definitions + instantiated aliases = reusable behavioral specifications:

```go
type StorageTestSuite[T Storage] struct {
    factory func() T
    store   T
}

func (s *StorageTestSuite[T]) BeforeEach(t *gotest.T) {
    s.store = s.factory()
}

func (s *StorageTestSuite[T]) TestPutAndGet(t *gotest.T) {
    t.When("key exists", func(w *gotest.T) {
        w.It("returns the value", func(it *gotest.T) {
            s.store.Put("key", "value")
            result, err := s.store.Get("key")
            gotest.NoError(it, err)
            gotest.Equal(it, result, "value")
        })
    })

    t.When("key does not exist", func(w *gotest.T) {
        w.It("returns ErrNotFound", func(it *gotest.T) {
            _, err := s.store.Get("nonexistent")
            gotest.ErrorIs(it, err, ErrNotFound)
        })
    })
}

type MemoryStorageTestSuite = StorageTestSuite[*MemoryStorage]
type RedisStorageTestSuite = StorageTestSuite[*RedisStorage]
```

**Spec output:**

```
MemoryStorage (conformance: Storage)
  PutAndGet
    when key exists
      ✓ returns the value
    when key does not exist
      ✓ returns ErrNotFound

RedisStorage (conformance: Storage)
  PutAndGet
    when key exists
      ✓ returns the value
    when key does not exist
      ✓ returns ErrNotFound
```

Each alias produces an independent conformance report. A failing test in one implementation doesn't affect others.

### Resource Lifecycle Guarantees

The generated code provides iron-clad guarantees:

1. `AfterAll` is registered via `t.Cleanup` BEFORE `BeforeAll` runs
2. `t.Cleanup` runs in LIFO order, so user-registered cleanups in `BeforeAll` run before `AfterAll`
3. `AfterEach` is `defer`-ed, so it runs even on `t.Fatal()` / `runtime.Goexit()`
4. In parallel suites, `wg.Wait()` completes before `AfterAll` — all tests finish before shared resources are torn down
5. `t.Fatal()` in `BeforeAll` skips the entire suite (standard Go behavior)
6. `t.Skip()` in `BeforeAll` marks the suite as skipped

---

## Non-Goals

These are features we deliberately will not build. Each has a reason.

### Test dependency ordering

Tests that depend on other tests are brittle. Each test should set up its own preconditions via `BeforeEach`. If Test B requires state from Test A, that state belongs in `BeforeEach`, not in Test A's side effects. We will not add `@DependsOn` or ordering guarantees beyond declaration order.

### Mocking framework

Mocking is orthogonal to test organization. `gomock`, `mockery`, `moq`, and counterfeiter all generate mock implementations that work inside suites unchanged. Adding mocking to `go-test` would bloat the tool and compete with mature solutions. The suite struct's fields are the injection points — that's enough.

### Decorator / annotation syntax

Go doesn't have decorators or annotations. Struct tags could work (`gotest:"parallel"`) but violate principle 2 (the naming IS the API). Naming conventions are grepped, autocompleted, and understood at a glance. Struct tags are hidden in backtick strings and require documentation lookup.

### Runtime suite registration

`suite.Run(t, new(MySuite))` is testify's approach. It works but requires a `func Test*` boilerplate wrapper per suite. The entire point of `go-test` is to generate that boilerplate. Adding runtime registration would create two ways to do the same thing.

### Cross-package suite inheritance

A suite in package `bar` embedding a suite from package `foo` would inherit `foo`'s lifecycle hooks. This breaks Go's package isolation model, creates import cycles, and makes test behavior depend on transitive dependencies. Suite inheritance works within a package (including test packages). Cross-package sharing should use helper functions, not suite embedding.

### Replacing `go test` output

The `spec` subcommand and `--spec` flag are post-processing views over `go test -json` data. They add a layer on top; they never suppress or replace the underlying `go test` output. When `gotest ./... -v` runs in default mode, the output is byte-identical to what `go test ./... -v` would produce.

---

## Architecture Evolution

### Phase 1: Current (in progress)

```
cmd/gotest → gotestrunner → gotestgen → gotestast
                                         └── templates
pkg/gotest
  ├── T, Assert (fluent), It
  ├── Equal, NoError, ErrorIs, ... (functional assertions)
  └── internal/assert (~300 lines, pure stdlib, zero deps)
```

Single pipeline: load → collect → reduce ��� render. Templates are Go `text/template`. Package loading via `golang.org/x/tools/go/packages`. Assertion library reimplemented from scratch — no vendored testify, no `go-spew`, no `go-difflib`.

### Phase 2: Scaffold + Spec + CLI Redesign

```
cmd/gotest
  ├── run (default)   → gotestrunner → gotestgen → gotestast
  ├── scaffold        → gotestast (reverse: type → suite skeleton)
  ├── spec            → go test -json → spec renderer
  ├── coverage        → gotestast + packages.Load (static comparison)
  ├── clean           → filesystem scan
  ├── generate        → gotestgen (no test execution)
  └── migrate         → AST transform (testify → go-test)

pkg/gotest (T, Assert, It, When, Each, Eventually, Consistently, MatchSnapshot + assertion functions)
```

The `scaffold` subcommand reuses the same `packages.Load` infrastructure as the generator but produces skeleton test files instead of harness files. The `spec` subcommand wraps `go test -json` and maps results back to the suite→method→It/When hierarchy. The `coverage` subcommand compares production API surfaces against suite inventories.

`pkg/gotest` grows with `When()`, `Each()`, `Eventually()`, `Consistently()`, and `MatchSnapshot()` — all runtime features that don't affect code generation.

### Phase 3: Watch Mode + Linter + Nested Suites

```
cmd/gotest
  └── watch → fsnotify → re-run pipeline on change

cmd/gotest-lint → go/analysis analyzers

gotestast: DetermineTestSuite learns embedding detection
gotestgen: renderer chains parent/child lifecycle hooks
templates: nested lifecycle template variant
```

### Key Architectural Invariant

The pipeline is always: **static analysis → code generation → standard `go test`**. No runtime component grows beyond the thin `gotest.T` wrapper. If a feature can't be implemented as either (a) generated code, (b) a method on `gotest.T`, or (c) post-processing of `go test -json`, it doesn't belong in this project.

---

## Success Metrics

How we know the project is achieving its vision:

1. **Adoption:** A developer can go from `go install` to running their first suite in under 2 minutes — `gotest scaffold` makes this possible without reading docs
2. **Transparency:** Running `go test` directly on a package with generated files produces identical output to running via `gotest` — the tool is truly a wrapper, not a replacement
3. **Trust:** The generated code is readable enough that developers can debug test failures by reading it — it should look like code they would write, not compiler output
4. **Specification:** `gotest spec ./...` produces a behavioral document that a non-developer (PM, QA) can read and understand what the system does
5. **Performance:** The generation + cleanup overhead is under 500ms for a 100-package project — the tool should feel instant
6. **Safety:** There is no scenario where generated files are orphaned without a warning on next run — the cleanup guarantees are iron-clad
7. **Compatibility:** Every `go test` flag works identically through `gotest` — `-race`, `-cover`, `-count`, `-run`, `-json`, `-short`, `-timeout`, `-cpu`, `-parallel`, `-bench`, `-v` — all of them, without the tool knowing about them
