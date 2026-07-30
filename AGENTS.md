# gotest — Agent Reference

gotest is a code-generation framework for Go test suites.
You write suite structs with lifecycle methods and test cases; the `gotest generate` command produces the test harness (test functions, fixture initialization, lifecycle wiring).
The generated code is not hand-edited.

All assertions accept both `*gotest.T` and `*testing.T` as first argument.

## Rules

These override any default instincts from stdlib `testing` or other Go test frameworks.

**Never call `t.T().Helper()` when using gotest assertions.**
gotest automatically resolves assertion failures to the correct call site in your test code.
Calling `t.T().Helper()` is counterproductive — it removes useful location information from failure output.
Without it, failures show both where the assertion was called (inside the helper) and where the helper was called (in the test).
This applies to all gotest assertion functions (`gotest.Equal`, `gotest.True`, etc.).
If you pass `t.T()` to third-party code that uses Go's standard `t.Errorf`, Go's `t.Helper()` convention applies as normal.

**Never call `t.T().Cleanup()` inside suite methods.**
Use `BeforeAll`/`AfterAll` for suite-level resources and `BeforeEach`/`AfterEach` for per-test resources.
The generated harness manages cleanup ordering (LIFO).
Calling `t.T().Cleanup()` inside suite methods breaks the predictable lifecycle.

**Never call `t.T().Fatalf()` in reusable test helpers.**
Use gotest assertions instead.
Helpers that call `t.T()` panic inside `Eventually`/`Consistently` because the collecting T has no underlying `*testing.T`.
This is the most common source of runtime panics in gotest code.

**Never use `t.T().Skip()` for environment gating.**
Use `SuiteGuard() string` instead.
SuiteGuard runs before the suite's config resolution, `BeforeAll`, and every test method — the suite's own setup never executes.
(Package/shared fixture setup currently still runs before the guard.)
`t.T().Skip()` inside `BeforeAll` runs even later, after suite setup has started.

**`Nil`/`NotNil` are for non-comparable nilables only.**
They carry a runtime type guard: slices, maps, funcs, channels, pointers, interfaces, and unsafe pointers are accepted; any other type fails with a type-guard error (the `assertion-type-guard` lint rule catches misuse statically).
Use `Zero`/`NotZero` for comparable nilables (pointers, interfaces, channels).
For slices and maps, prefer `Empty`/`NotEmpty` — a nil slice and an empty slice are equivalent for most test intent; reach for `Nil`/`NotNil` only when the nil vs empty distinction matters.

**`HasPrefix`/`HasSuffix` assertions do not exist.**
Use `Regexp(t, "^prefix", str)` or `Regexp(t, "suffix$", str)`.

**Prefer specific assertions over generic boolean checks.**
The linter enforces assertion simplification.
`True(t, a == b)` → `Equal(t, a, b)`. `True(t, err != nil)` → `Error(t, err)`. `True(t, len(x) == 0)` → `Empty(t, x)`. `Equal(t, true, v)` → `True(t, v)`. `Equal(t, nil, v)` → `Zero(t, v)`. `Len(t, x, 0)` → `Empty(t, x)`.
The more specific assertion produces better failure messages.

**Always pair `BeforeAll` with `AfterAll` in suites.**
If `BeforeAll` acquires resources, add an `AfterAll` to release them.
The linter flags unpaired `BeforeAll` as a likely resource leak.

## Assertions

All assertion functions call `FailNow()` on failure (fatal — test stops immediately).
All accept optional trailing `msgAndArgs ...any` for custom error messages.

### Unconditional Failure

```go
gotest.Fail(t, msgAndArgs ...any)                    // immediately fails with message
```

Use in unreachable branches (`default` in exhaustive switches, after calls that must not return).
Prefer over `gotest.True(t, false, "msg")`.

### Equality

```go
gotest.Equal[V any](t, expected, actual V)          // deep equality (reflect.DeepEqual)
gotest.NotEqual[V any](t, expected, actual V)        // deep inequality
```

### Boolean

```go
gotest.True(t, value bool)
gotest.False(t, value bool)
```

### Zero Value (also covers nil)

```go
gotest.Zero[V comparable](t, value V)               // value == zero value for type
gotest.NotZero[V comparable](t, value V)             // value != zero value for type
```

`comparable` includes: pointers, interfaces, channels, numerics, strings, bools, structs of comparables, arrays of comparables.
It excludes: slices, maps, functions.

For nil checks on slices, maps, or functions: `gotest.NotNil(t, x)` / `gotest.Nil(t, x)` (never `True(t, x != nil)` — the `assertion-simplify` rule rewrites it).

### Collections

```go
gotest.Empty(t, object any)                          // nil, or len == 0 (string, slice, map, chan, array)
gotest.NotEmpty(t, object any)
gotest.Len(t, object any, length int)                // exact length check
gotest.Contains(t, haystack, needle any)             // substring (string), element (slice/array), key (map)
gotest.NotContains(t, haystack, needle any)
gotest.ElementsMatch[V comparable](t, a, b []V)     // same elements, any order
gotest.Subset[V comparable](t, list, subset []V)     // every subset element exists in list
```

### Errors

```go
gotest.NoError(t, err error)                         // err == nil
gotest.Error(t, err error)                           // err != nil
gotest.ErrorIs(t, err, target error)                 // errors.Is(err, target)
gotest.ErrorAs[E error](t, err error) E              // errors.As — returns matched error
gotest.ErrorContains(t, err error, substr string)    // err != nil && strings.Contains(err.Error(), substr)
```

### Comparison

```go
gotest.Greater[V cmp.Ordered](t, a, b V)            // a > b
gotest.GreaterOrEqual[V cmp.Ordered](t, a, b V)     // a >= b
gotest.Less[V cmp.Ordered](t, a, b V)               // a < b
gotest.LessOrEqual[V cmp.Ordered](t, a, b V)        // a <= b
```

### Strings & Patterns

```go
gotest.Regexp[P regexpPattern](t, rx P, str string)  // accepts string pattern or *regexp.Regexp
```

Use `"^prefix"` for HasPrefix, `"suffix$"` for HasSuffix, `"^exact$"` for exact match.

### Numeric

```go
gotest.InDelta[V numeric](t, expected, actual V, delta float64)
```

### JSON

```go
gotest.JSONEq(t, expected, actual any)  // structural JSON equality
                                         // accepts: string, []byte, json.RawMessage, io.Reader, or any marshalable value
```

### Time

```go
gotest.TimeWithin(t, expected, actual time.Time, tolerance time.Duration)
gotest.TimeIsNow(t, ts time.Time, tolerance time.Duration)
```

### Panics

```go
gotest.Panics(t, f func()) any  // asserts f panics, returns recovered value
```

### Snapshots

```go
gotest.MatchSnapshot(t, value any, name ...string)   // compare value against stored snapshot
```

Snapshots are stored in `testdata/__snapshots__/<TestName>.snap` (e.g. `TestMyTestSuite.snap`; suffix `_ext` for external `_test` packages) next to the test file.
On first run (or with `--update-snapshots`), the snapshot is created.
On subsequent runs, the value is compared against the stored snapshot.
The optional `name` disambiguates multiple snapshots within the same test case.
Snapshot entries are written in deterministic order and are thread-safe — parallel test methods can use `MatchSnapshot` without coordination.

## Eventually & Consistently

Package-level functions for async polling. Both use `*gotest.R` — an assertion recorder that captures failures without propagating them.

```go
gotest.Eventually(t, 5*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
    gotest.Equal(poll, "ready", getStatus())
})

gotest.Consistently(t, 2*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
    gotest.True(poll, isHealthy())
})
```

There are no method forms on `*T`. Only package-level `gotest.Eventually()` and `gotest.Consistently()` exist.

**Critical constraint:** Inside the callback, `poll` is a `*gotest.R`, not a `*gotest.T`.
`poll` has no `T()` method. All code in the callback must use gotest assertions only.
Any helper that calls `t.T()` will panic.
This is by design — the recorder intercepts failures without aborting, enabling retry semantics.

A panic in the callback fails the test and is reported with the stack from where it
happened. The callback runs on its own goroutine, so the panic is carried back to the
test's goroutine before being re-raised — `AfterEach`, `AfterAll` and fixture teardown all
still run.

## Goroutines Started by Tests

A panic on a goroutine the test starts is unrecoverable: Go terminates the process
without running any other goroutine's deferred work, so no `AfterEach`, no `AfterAll`
and no fixture teardown happens. No framework can guard a goroutine it did not create.

Use `gotest.Go` to start one instead. It captures the panic with the stack from where
it happened and re-raises it on the test's own goroutine, where it is reported like any
other test panic and every cleanup still runs.

```go
wait := gotest.Go(t, func() { srv.Serve(l) })
defer wait()
```

The wait is also registered as test cleanup, so a forgotten `wait()` still gets one.

## Suite Conventions

### Suite types

```go
type MyServiceTestSuite struct { /* state */ }           // test suite
```

- Name must end with `TestSuite`
- All methods must use pointer receivers
- Must have at least one `Test*` method

### Test methods

```go
func (s *MyTestSuite) TestCreate(t *gotest.T) {}         // standard test method
func (s *MyTestSuite) TestRead(t *gotest.T) {}
```

Methods can also accept `*testing.T` for stdlib compatibility.

### Lifecycle methods

```go
func (s *MyTestSuite) BeforeAll(t *gotest.T)  {}  // once before all tests
func (s *MyTestSuite) AfterAll(t *gotest.T)   {}  // once after all tests (including parallel)
func (s *MyTestSuite) BeforeEach(t *gotest.T) {}  // before each test
func (s *MyTestSuite) AfterEach(t *gotest.T)  {}  // after each test
```

All are optional.
Execution order: BeforeAll -> (BeforeEach -> Test -> AfterEach)* -> AfterAll.
For parallel test cases, AfterAll waits for all parallel tests to complete.

**On failure and panic.**
`AfterAll` runs even when `BeforeAll` failed or panicked, so teardown may see partly
initialised state — guard it if `BeforeAll` can fail midway. Write `AfterAll` to release
whatever `BeforeAll` had reached, not to assume it finished.
The void `AfterEach` likewise runs even when `BeforeEach` panicked. The returning form
does not: with nothing returned there is no context to release, and the per-test state it
would clean up was never created.

**Returning BeforeEach (required for `Parallel: true`):**

When `SuiteConfig` enables `Parallel` on a suite with per-test state, `BeforeEach` must return a context struct pointer to give each parallel test its own isolated state (field-less suites, or suites whose only fields are fixture pointers, may be parallel without one). Test methods and `AfterEach` then accept this context as an additional parameter:

```go
type testCtx struct {
    db   *sql.DB
    repo *UserRepository
}

func (s *MyTestSuite) BeforeEach(t *gotest.T) *testCtx {
    db := setupTestDB()
    return &testCtx{db: db, repo: newUserRepository(db)}
}

func (s *MyTestSuite) AfterEach(t *gotest.T, ctx *testCtx) {
    ctx.db.Close()
}

func (s *MyTestSuite) TestCreate(t *gotest.T, ctx *testCtx) {
    gotest.NoError(t, ctx.repo.Create("alice"))
}
```

Without the returning form, the suite struct `s` is shared across parallel methods — writing to `s` fields is a data race.

### SuiteConfig (optional)

```go
func (s *MyTestSuite) SuiteConfig() gotest.SuiteConfig {
    return gotest.IntegrationSuiteConfig() // Timeout: 2m, SetupTimeout: 5m
}
```

Fields: `Timeout` (per-test deadline), `SetupTimeout`, `FailFast` (stop on first failure), `Parallel` (run methods concurrently).
Presets: `DefaultSuiteConfig()` (30s/30s), `IntegrationSuiteConfig()` (2m/5m).
The returned config is used as-is: a zero (or omitted) duration disables that deadline; without the marker method, `DefaultSuiteConfig()` (30s/30s) applies.
Compose from presets for defaults + overrides: `cfg := gotest.DefaultSuiteConfig(); cfg.Parallel = true; return cfg`.

A timeout does two things, and only one of them is defaulted.

It always bounds `t.Context()`, with the preset value when you declare nothing. That
costs nothing: code that watches its context gets canceled, code that does not carries
on.

It is also a budget, enforced by verdict — but only when **you** declare it. `Timeout`
cannot interrupt a running test, because Go has no way to stop another goroutine, so a
method that ignores `t.Context()` and outlives the budget is failed instead. That
failure is reported the moment the deadline passes, while the method is still running,
and is also written unbuffered so it survives a test that never returns at all.
A suite that never wrote a `Timeout` is not held to one; being failed against a number
you did not choose is not a verdict you can act on. Bounding the process remains
`go test -timeout`'s job either way.

`SetupTimeout` works the same way for `BeforeAll` and `AfterAll`, including for a setup
that hangs and never returns. A declared fixture `Timeout` likewise: an overrunning
fixture `BeforeAll` counts as a failed attempt, retried if `Retries` allows.

`Timeout` reaches `t.Context()` inside nested `When`, `It` and `Each` bodies too — a
nested behavior inherits the enclosing deadline, and its context is canceled when that
nested subtest ends. For a tighter budget in one place, use `gotest.NewTWithDeadline`.

### SuiteGuard (optional)

```go
func (s *MyTestSuite) SuiteGuard() string {
    if os.Getenv("DATABASE_URL") == "" {
        return "DATABASE_URL not set"
    }
    return "" // empty = run the suite
}
```

Returns empty string to run, non-empty to skip with that reason.
Runs before the suite's config, `BeforeAll`, and any test method (fixture setup currently still runs first).
Works for standalone and fixture-bound suites alike.

### Focus & Exclude prefixes

```go
type F_DebugThisTestSuite struct {}     // FOCUSED: only F_ suites run; all others skipped
type X_BrokenTestSuite struct {}        // EXCLUDED: never runs

func (s *MyTestSuite) F_TestSpecific(t *gotest.T) {}  // FOCUSED test case
func (s *MyTestSuite) X_TestBroken(t *gotest.T) {}    // EXCLUDED test case
```

- `F_` — focus. If any `F_` suite or test case exists, only focused items run. Causes CI failure with `gotest --ci`.
- `X_` — exclude. Always skipped. Compile-time decision (AST level). Use for permanently/temporarily disabled tests.

Focus and exclude apply independently at both suite and test case levels.

## Fixtures

### Package fixtures

Shared setup for suites in the same package. Name must end with `Fixture`.

```go
type DBFixture struct {
    Pool *pgxpool.Pool
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { /* start db, set f.Pool */ return nil }
func (f *DBFixture) AfterAll(ctx context.Context) error   { f.Pool.Close(); return nil }
func (f *DBFixture) BeforeEach(ctx context.Context) error { /* truncate tables */ return nil }
func (f *DBFixture) AfterEach(ctx context.Context) error  { return nil }
```

`BeforeAll` is required. All other methods are optional.
Fixture hooks use `(ctx context.Context) error` — different from suite hooks.

Suites wire fixtures via pointer fields:

```go
type UserTestSuite struct {
    DB *DBFixture
}
```

When both fixture and suite define `BeforeEach`/`AfterEach`, the fixture's hooks wrap the suite's:
fixture.BeforeEach -> suite.BeforeEach -> test -> suite.AfterEach -> fixture.AfterEach.

Fixtures compose via pointer fields (DAG — dependencies set up first):

```go
type APIFixture struct {
    DB    *DBFixture     // set up before APIFixture
    Cache *CacheFixture  // set up before APIFixture (parallel with DB)
}
```

### FixtureConfig (optional)

```go
func (f *DBFixture) FixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig() // Timeout: 5m, Retries: 1, RetryDelay: 5s
}
```

Presets: `DefaultFixtureConfig()` (2m timeout), `ContainerFixtureConfig()` (5m, 1 retry, 5s delay).

**Teardown failures are real failures.** A fixture `AfterAll` that returns an error or
panics fails the run — for package and shared fixtures alike. A panic in one fixture's
teardown is contained, so every other fixture still finishes releasing its resources.

`Retries` covers panics as well as returned errors — a `BeforeAll` that panics is
retried like one that fails. If a shared fixture's process dies before teardown, the
run fails: its `AfterAll` never ran and whatever it held is orphaned.

### Shared fixtures

Cross-package fixtures that run in a subprocess. Name must end with `SharedFixture`.
State transfers via JSON serialization — non-serializable fields are reconstructed via `Hydrate`.

```go
type PostgresSharedFixture struct {
    ConnStr string         // transfer field — serialized across processes
    Pool    *pgxpool.Pool  // local field — assigned in Hydrate, excluded from serialization
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error {
    f.ConnStr = startPostgres(ctx)
    return f.connect(ctx)
}
func (f *PostgresSharedFixture) AfterAll(ctx context.Context) error   { return stopPostgres() }
func (f *PostgresSharedFixture) Hydrate(ctx context.Context) error    { return f.connect(ctx) }
func (f *PostgresSharedFixture) Dehydrate(ctx context.Context) error  { f.Pool.Close(); return nil }

func (f *PostgresSharedFixture) connect(ctx context.Context) error {
    var err error
    f.Pool, err = pgxpool.New(ctx, f.ConnStr)
    return err
}
```

Fields assigned in `Hydrate` are automatically classified as local and excluded from serialization.

Shared fixture configuration uses a method named `SharedFixtureConfig()` (not `FixtureConfig()`) but returns the same `gotest.FixtureConfig` type:

```go
func (f *PostgresSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig() // 5m timeout, 1 retry
}
```

Suites reference shared fixtures via pointer fields (same as package fixtures).
Shared fixtures must not live in `internal/` packages.

### Generic test suites (contract testing)

Generic suite structs define contract tests once; type aliases instantiate them for each concrete type:

```go
type IndexContractTestSuite[T Indexable] struct{}

func (s *IndexContractTestSuite[T]) SuiteConfig() gotest.SuiteConfig {
    return gotest.SuiteConfig{Parallel: true}
}

func (s *IndexContractTestSuite[T]) TestEmptyIndex(t *gotest.T) {
    idx := newIndex[T]()
    gotest.Empty(t, idx.Search("anything"))
}

// Type aliases create concrete suites the framework discovers:
type ArticleIndexTestSuite = IndexContractTestSuite[Article]
type ProductIndexTestSuite = IndexContractTestSuite[Product]
```

Each type alias produces a separate suite that runs independently.

## Parallel Semantics

**Suite-level**: Each suite runs as a separate subprocess — full process isolation. This is automatic.

**Method-level**: Opt-in via `SuiteConfig{Parallel: true}`.
All test methods within the suite run concurrently.
Suite struct state `s` is shared — writing to `s` fields from parallel methods is a data race.
Use a returning `BeforeEach` to give each method its own isolated state.

## Other T Methods

```go
t.T() *testing.T                                    // access underlying testing.T (panics inside Eventually)
t.Context() context.Context                          // test context (respects deadline from SuiteConfig)
t.Skipf(format, args...)                             // skip the test with a formatted reason
t.Setenv(key, value)                                 // set env var, restored after test
t.TempDir() string                                   // create a temp dir, cleaned up after test
t.It("description", func(it *gotest.T) { ... })     // BDD-style subtest
t.When("condition", func(w *gotest.T) { ... })       // BDD-style subtest
```

Table-driven tests with `Each`:
```go
for t, entry := range gotest.Each(t, entries) {
    gotest.Equal(t, entry.Expected, Compute(entry.Input))
}
```
Subtest names are derived from the entry struct's `Desc` or `Name` string field (if present), otherwise `#0`, `#1`, etc.

## Utilities

```go
gotest.Must[T any](val T, ok any) T  // panics if ok is error/false; returns val otherwise
                                      // useful in test setup: db := gotest.Must(sql.Open(...))
```
