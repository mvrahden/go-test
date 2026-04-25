# go-test Superspec

## Vision

Go's testing framework is deliberately minimal — and that's a strength. But the gap between `func TestX(t *testing.T)` and a well-organized, maintainable test suite is filled entirely by manual boilerplate. `go-test` closes that gap through code generation: you write structs, name them well, and the tool handles the rest. No runtime dependencies. No reflection. No lock-in. Just standard Go tests with lifecycle management, structured organization, and development-time ergonomics that make testing a pleasure rather than a chore.

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

## Tier 1 — Complete the Foundation

*What's needed to call this "production-ready."*

### 1.1 Assertion API: Complete and Type-Safe

**Why:** The current fluent API (`t.Assert(v).IsEqualTo(w)`) is expressive but untyped — comparing a `string` to an `int` compiles. The `Contains` string case is a no-op. `ContainsAll`/`ContainsAny` panic. Developers won't trust an assertion library that's half-built.

**What:** Two complementary APIs, both complete:

**Fluent API** — discoverable, reads like prose, for quick assertions:

```go
t.Assert(result).IsEqualTo(expected)
t.Assert(items).HasLength(3)
t.Assert(err).IsZero()
```

Complete the existing methods:
- `Contains(v)` — fix the string case (use `strings.Contains`)
- `ContainsAll(v...)` — check all elements are present
- `ContainsAny(v...)` — check at least one element is present
- `IsNotEqualTo(v)` — negated equality
- `IsNotZero()` — negated zero check
- `IsNotEmpty()` — negated empty check
- `IsNil()` / `IsNotNil()` — nil checks with proper pointer/interface handling
- `IsError(target)` — `errors.Is` check
- `ErrorContains(substr)` — error message substring check
- `Panics()` / `PanicsWithValue(v)` — panic recovery assertions
- `Satisfies(func(v any) bool)` — custom predicate

**Functional API** — type-safe, familiar to testify users, for production test suites:

```go
require.Equal(t, got, want)           // T constrained to comparable
require.DeepEqual(t, got, want)       // any type, uses reflect.DeepEqual
require.Contains(t, haystack, needle) // generic over slice element type
require.ErrorIs(t, err, target)
require.NoError(t, err)
require.Len(t, collection, expected)
require.True(t, condition)
require.Nil(t, value)
```

Both APIs accept `*gotest.T`. The functional API uses Go generics for compile-time type safety — `require.Equal[int](t, "foo", 42)` is a compile error, not a runtime surprise.

**Diff output for equality failures:**

```
require.Equal failed:
  expected: map[string]int{"a": 1, "b": 2, "c": 3}
  actual:   map[string]int{"a": 1, "b": 5, "c": 3}
  diff:
    map[string]int{
        "a": 1,
    -   "b": 2,
    +   "b": 5,
        "c": 3,
    }
```

Use `go-cmp` or a minimal built-in differ for structured diff output. Pointer addresses and unexported fields are handled gracefully.

### 1.2 CLI Distribution

**Why:** `go run github.com/mvrahden/go-test/cmd/testsuite ./...` is 40 characters of incantation. Developers need `gotest ./...`.

**What:**

- **Binary name:** `gotest` (or `go-test` if `gotest` conflicts)
- **Install:** `go install github.com/mvrahden/go-test/cmd/gotest@latest`
- **Invocation:** `gotest ./... -v -race -count=1` — identical argument order to `go test`
- **Shell alias guidance:** `alias got='gotest'` for the truly lazy
- **Version:** `gotest -ƒƒ.version` prints version, Go version, OS/arch
- **goreleaser** for cross-platform binaries and GitHub releases

### 1.3 README and Examples

**Why:** No README = no adoption. The README is the product for open-source tools.

**What:**

```
# gotest

Structured test suites for Go. Zero runtime dependencies.

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

## Tier 2 — Developer Experience

*What makes developers choose this over testify/suite.*

### 2.1 Watch Mode

**Why:** The tightest feedback loop wins. A developer iterating on a feature wants to save a file and see test results instantly, without switching windows to run a command.

**What:**

```
gotest -ƒƒ.watch ./...
```

Behavior:
1. Initial run: full generate → test → cleanup cycle
2. Watch filesystem for `.go` file changes (via `fsnotify`)
3. On change: re-run the generate → test → cleanup cycle for affected packages
4. Display: clear terminal, show results, show "watching..." prompt
5. On Ctrl+C: cleanup and exit

**Package-scoped re-runs:** When `pkg/foo/bar.go` changes, re-run `./pkg/foo/...` not the entire pattern. Use Go's package graph to determine the minimal affected set.

**Focus integration:** During watch mode, `F_`-prefixed suites create an extremely tight loop — only the focused tests re-run on every save. The workflow:

1. `gotest -ƒƒ.watch ./...` in a terminal
2. Prefix the suite you're working on with `F_`
3. Save → only that suite runs → instant feedback
4. Remove the `F_` prefix → all suites run → confirm nothing broke
5. Commit

**Debouncing:** Multiple rapid file saves (e.g., editor auto-save, `goimports`) trigger a single re-run after 200ms of quiet.

### 2.2 Data-Driven Testing: `t.Each()`

**Why:** Table-driven tests are Go's most common pattern but require verbose boilerplate: define a struct, build a slice, range over it, call `t.Run`. `t.Each` reduces this to two lines.

**What:**

```go
func (s *Suite) TestValidation(t *gotest.T) {
    t.Each([]struct {
        Input    string
        Expected bool
    }{
        {"valid@email.com", true},
        {"not-an-email", false},
        {"", false},
    }, func(it *gotest.T, tc struct{ Input string; Expected bool }) {
        result := s.validator.IsValid(tc.Input)
        require.Equal(it, result, tc.Expected)
    })
}
```

Each table entry becomes a `t.Run` subtest named by index (or by a `Name` field if present in the struct). This is a runtime feature on `gotest.T` — no code generation changes needed.

**Named entries:** If the table struct has a `Name string` or `Desc string` field, use it as the subtest name:

```go
t.Each([]struct {
    Desc     string
    Input    int
    Expected int
}{
    {"zero", 0, 0},
    {"positive", 5, 25},
    {"negative", -3, 9},
}, func(it *gotest.T, tc struct{ Desc string; Input, Expected int }) {
    require.Equal(it, s.square(tc.Input), tc.Expected)
})
```

Produces:

```
=== RUN   TestMathTestSuite/TestSquare/zero
=== RUN   TestMathTestSuite/TestSquare/positive
=== RUN   TestMathTestSuite/TestSquare/negative
```

### 2.3 Snapshot Testing: `t.MatchSnapshot()`

**Why:** Golden-file testing is powerful but manual. Developers copy-paste expected output, and it drifts. Snapshot testing auto-manages expectations: the first run creates the snapshot, subsequent runs compare against it, and a flag updates them.

**What:**

```go
func (s *Suite) TestJSONOutput(t *gotest.T) {
    result := s.sut.ToJSON()
    t.MatchSnapshot(result)
}
```

Behavior:
- Snapshots stored in `testdata/__snapshots__/<TestName>.snap` (per-suite directory)
- First run with no snapshot: create it, pass the test, print "created snapshot"
- Subsequent runs: compare against snapshot, fail with diff on mismatch
- Update: `gotest -ƒƒ.update-snapshots ./...` regenerates all snapshots
- Format: snapshots are plain text (strings) or formatted Go values (structs, maps)

**Multi-snapshot per test:**

```go
func (s *Suite) TestMultiFormat(t *gotest.T) {
    t.MatchSnapshot(s.sut.ToJSON(), "json")
    t.MatchSnapshot(s.sut.ToYAML(), "yaml")
}
```

Each call gets a unique snapshot file keyed by the optional name parameter.

### 2.4 Improved Error Context

**Why:** When a test fails deep in a suite lifecycle, the developer needs to know which suite, which test case, and which assertion failed — without reading a stack trace.

**What:**

Assertion failures include full context:

```
--- FAIL: TestUserServiceTestSuite/TestCreateUser (0.003s)
    require.Equal failed:
      location:  user_service_test.go:47
      expected:  "active"
      actual:    "pending"
      diff:      -"active" +"pending"
```

Implementation: `require` functions use `t.Helper()` to report the caller's file:line. The diff renderer handles strings, structs, maps, and slices. Pointer addresses are never shown in diffs.

---

## Tier 3 — Advanced Patterns

*What makes this the definitive Go testing tool.*

### 3.1 Nested Suites via Embedding

**Why:** Real-world test organization is hierarchical. Testing a `UserService` involves "create," "update," "delete" — each with their own setup. Flat suites force either one mega-struct or scattered independent suites with duplicated setup.

**What:** A suite that embeds another suite inherits its lifecycle hooks:

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

// Nested suite — inherits db and svc from parent
type UserCreateTestSuite struct {
    UserServiceTestSuite
    user *User
}

func (s *UserCreateTestSuite) BeforeEach(t *gotest.T) {
    s.user = &User{Name: "test"}
}

func (s *UserCreateTestSuite) TestCreatesUser(t *gotest.T) {
    err := s.svc.Create(s.user)
    require.NoError(t, err)
}

func (s *UserCreateTestSuite) TestRejectsDuplicate(t *gotest.T) {
    s.svc.Create(s.user)
    err := s.svc.Create(s.user)
    require.ErrorIs(t, err, ErrDuplicate)
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

**Detection:** The generator inspects struct fields. If a field's type is another recognized test suite, it's a parent-child relationship. The child suite's generated `Test*` function chains the parent's lifecycle hooks.

**Naming:** The generated test function is `TestUserCreateTestSuite`. The parent suite does NOT generate its own `Test*` function unless it has its own test case methods (methods not inherited by children).

**Constraint:** Only single-level embedding is supported initially. Multi-level embedding (grandparent → parent → child) is a natural extension but not required for v1.

### 3.2 Contract Testing via Generic Suites

**Why:** Go interfaces define contracts. Testing those contracts against every implementation is tedious — the same tests are copy-pasted with different concrete types. Generic suites + type aliases solve this.

**What:**

```go
// Define the contract test suite once
type StorageTestSuite[T Storage] struct {
    factory func() T
    store   T
}

func (s *StorageTestSuite[T]) BeforeEach(t *gotest.T) {
    s.store = s.factory()
}

func (s *StorageTestSuite[T]) TestPutAndGet(t *gotest.T) {
    s.store.Put("key", "value")
    result, err := s.store.Get("key")
    require.NoError(t, err)
    require.Equal(t, result, "value")
}

func (s *StorageTestSuite[T]) TestGetMissing(t *gotest.T) {
    _, err := s.store.Get("nonexistent")
    require.ErrorIs(t, err, ErrNotFound)
}

func (s *StorageTestSuite[T]) TestDelete(t *gotest.T) {
    s.store.Put("key", "value")
    s.store.Delete("key")
    _, err := s.store.Get("key")
    require.ErrorIs(t, err, ErrNotFound)
}

// Instantiate for each implementation — each gets the full test battery
type MemoryStorageTestSuite = StorageTestSuite[*MemoryStorage]
type RedisStorageTestSuite = StorageTestSuite[*RedisStorage]
type SQLStorageTestSuite = StorageTestSuite[*SQLStorage]
```

This pattern is already supported by the current generic suite implementation. The superspec contribution is recognizing this as a first-class use case and providing:

- **Factory initialization:** A convention for setting the `factory` field. Options:
  - Constructor function: `func NewMemoryStorageTestSuite() *MemoryStorageTestSuite`
  - BeforeAll hook on the alias (not possible with Go aliases)
  - Embedding with a constructor struct

- **Documentation and examples:** The `examples/contract_suite/` package demonstrates the pattern with a minimal `Storage` interface and two implementations.

### 3.3 Suite-Scoped Context and Shared Resources

**Why:** Database connections, HTTP servers, Docker containers are expensive. Suites need to share resources across tests without recreating them per test case. The current `BeforeAll`/`AfterAll` handles this, but the pattern should be explicit and safe.

**What:**

A convention for suite-scoped resources:

```go
type IntegrationTestSuite struct {
    db     *sql.DB
    server *httptest.Server
}

func (s *IntegrationTestSuite) BeforeAll(t *gotest.T) {
    var err error
    s.db, err = sql.Open("postgres", os.Getenv("TEST_DATABASE_URL"))
    require.NoError(t, err)

    handler := NewHandler(s.db)
    s.server = httptest.NewServer(handler)
}

func (s *IntegrationTestSuite) AfterAll(t *gotest.T) {
    s.server.Close()
    s.db.Close()
}
```

The generated code guarantees `AfterAll` runs via `t.Cleanup` even if `BeforeAll` panics partway through (cleanup for the already-initialized resources). This is already the behavior — the superspec contribution is documenting the resource lifecycle guarantees:

1. `AfterAll` is registered via `t.Cleanup` BEFORE `BeforeAll` runs
2. `t.Cleanup` runs in LIFO order, so user-registered cleanups in `BeforeAll` run before `AfterAll`
3. `AfterEach` is `defer`-ed, so it runs even on `t.Fatal()` / `runtime.Goexit()`
4. In parallel suites, `wg.Wait()` completes before `AfterAll` — all tests finish before shared resources are torn down

### 3.4 Setup Validation

**Why:** A suite with `BeforeAll` that fails silently (returns without error, but the resource is nil) causes cryptic nil-pointer panics in every test case. The developer debugs 20 test failures when the root cause is one setup failure.

**What:**

If `BeforeAll` calls `t.Fatal()` or `t.Skip()`, the generated code skips all test cases in the suite. This is already the behavior (since `t.Fatal` in a parent test prevents subtests from running). The superspec makes it explicit and adds guidance:

- Document that `t.Fatal()` in `BeforeAll` skips the entire suite (standard Go behavior via `runtime.Goexit()`)
- Document that `t.Skip()` in `BeforeAll` marks the suite as skipped
- Recommend `require.NoError(t, err)` in `BeforeAll` for setup validation

---

## Tier 4 — Ecosystem

*What makes this indispensable in the Go community.*

### 4.1 CI Integration

**Why:** The tool must work in CI without special configuration. `go install` + `gotest ./...` should be the entire CI test step.

**What:**

- **GitHub Actions action:** `uses: mvrahden/setup-gotest@v1` installs the binary and caches it
- **Exit codes:** Match `go test` exit codes exactly (0 = pass, 1 = test failure, 2 = build error)
- **`-ƒƒ.ci` flag:** Fails if any `F_`-prefixed suites or test cases exist — prevents focused tests from being committed. This is the CI safety net for the focus workflow.

```yaml
# .github/workflows/test.yml
- uses: mvrahden/setup-gotest@v1
- run: gotest -ƒƒ.ci ./... -v -race
```

The `-ƒƒ.ci` check is a static analysis step that runs before generation. It scans for `F_` prefixes in suite names and method names and fails immediately with a clear message:

```
FAIL: focus prefix detected — remove F_ before committing:
  pkg/user/user_test.go:12  type F_UserTestSuite
  pkg/user/user_test.go:28  func F_TestCreate
```

### 4.2 IDE Experience

**Why:** Developers live in their editor. If the IDE can't run a single suite method with one click, adoption suffers.

**What:**

The generated `func Test*(t *testing.T)` functions are already discovered by every Go IDE. The subtest names match method names exactly, so `-run TestMyTestSuite/TestCreate` works from the IDE's test runner.

Additional opportunities:
- **VS Code snippet:** `gotest-suite` scaffold that expands to a complete suite struct
- **LSP-aware naming:** Suite method names follow patterns that `gopls` auto-completes (all exported, all start with known prefixes)
- **Stale file detection:** If a generated `ƒƒ_*` file exists in the working tree (left by `-ƒƒ.internal.debug` or a crash), the tool prints a warning at startup rather than silently using stale code

### 4.3 Go Generate Integration

**Why:** Some teams prefer `go generate` workflows over custom CLI wrappers. The tool should support both.

**What:**

```go
//go:generate gotest -ƒƒ.generate ./...
```

The `-ƒƒ.generate` flag runs only the generation step (no test execution, no cleanup). The developer then runs `go test` normally. This is useful for:
- Teams that want generated files checked in (inspectable, diffable)
- Pre-commit hooks that validate generated files are up-to-date
- Build systems that separate generation from execution

The `-ƒƒ.generate` and `-ƒƒ.clean` flags are complementary:
- `gotest -ƒƒ.generate ./...` — generate files, leave them
- `go test ./...` — run tests (generated files are present)
- `gotest -ƒƒ.clean ./...` — remove generated files

### 4.4 Linter: `gotest-lint`

**Why:** Naming conventions are the API, but naming mistakes are silent. A method named `BeforAll` (typo) is just a regular method — the generator ignores it, and the developer's setup code never runs.

**What:**

A static analysis tool (standalone or `golangci-lint` plugin) that warns about:

- **Likely typos:** `BeforAll`, `AfterEeach`, `Testfoo` — methods on suite structs that are close to but don't match known patterns
- **Wrong receiver type:** Value receivers on suite methods (already caught by generator, but the linter catches it before running)
- **Missing lifecycle hook:** A suite with `BeforeAll` but no `AfterAll` — may leak resources (warning, not error)
- **Focused tests committed:** `F_` prefix in non-test-infrastructure code
- **Orphaned generated files:** `ƒƒ_*` files checked into version control

---

## Non-Goals

These are features we deliberately will not build. Each has a reason.

### Custom test reporters

`go test -json` produces machine-readable output. Tools like `gotestfmt`, `tparse`, and IDE integrations consume it. Building a custom reporter means either replacing `go test` output (breaks principle 1) or post-processing it (already solved by the ecosystem). We pass through and stay invisible.

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

---

## Architecture Evolution

### Phase 1: Current (in progress)

```
cmd/gotest → gotestrunner → gotestgen → gotestast
                                        └── templates
pkg/gotest (T, Assert, It)
```

Single pipeline: load → collect → reduce → render. Templates are Go `text/template`. Package loading via `golang.org/x/tools/go/packages`.

### Phase 2: Watch Mode + Linter

```
cmd/gotest → gotestrunner → gotestgen → gotestast
    │                                    └── templates
    ├── watcher (fsnotify)
    └── linter (go/analysis)

pkg/gotest (T, Assert, It, Each, MatchSnapshot)
```

The watcher wraps the existing pipeline in a file-change loop. The linter is a standalone `go/analysis` analyzer that can run independently or as a `golangci-lint` plugin.

`pkg/gotest` grows with `Each()` and `MatchSnapshot()` — these are runtime features that don't affect code generation.

### Phase 3: Nested Suites

```
gotestast: DetermineTestSuite learns embedding detection
gotestgen: renderer chains parent/child lifecycle hooks
templates: nested lifecycle template variant
```

The AST analysis phase detects struct fields whose types are other recognized suites. The renderer produces chained lifecycle calls. The template gains a `{{ if $ts.ParentSuite }}` branch.

### Key Architectural Invariant

The pipeline is always: **static analysis → code generation → standard `go test`**. No runtime component grows beyond the thin `gotest.T` wrapper. If a feature can't be implemented as either (a) generated code or (b) a method on `gotest.T`, it doesn't belong in this project.

---

## Success Metrics

How we know the project is achieving its vision:

1. **Adoption:** A developer can go from `go install` to running their first suite in under 2 minutes with no documentation beyond the README
2. **Transparency:** Running `go test` directly on a package with generated files produces identical output to running via `gotest` — the tool is truly a wrapper, not a replacement
3. **Trust:** The generated code is readable enough that developers can debug test failures by reading it — it should look like code they would write, not compiler output
4. **Performance:** The generation + cleanup overhead is under 500ms for a 100-package project — the tool should feel instant
5. **Safety:** There is no scenario where generated files are orphaned without a warning on next run — the cleanup guarantees are iron-clad
6. **Compatibility:** Every `go test` flag works identically through `gotest` — `-race`, `-cover`, `-count`, `-run`, `-json`, `-short`, `-timeout`, `-cpu`, `-parallel`, `-bench`, `-v` — all of them, without the tool knowing about them
