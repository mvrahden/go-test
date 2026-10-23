# gotest — Agent Reference

gotest is a code-generation framework for Go test suites.
You write suite structs with lifecycle methods and test cases; the `gotest generate` command produces the test harness (test functions, fixture initialization, lifecycle wiring).
The generated code is not hand-edited.
Every behavioral promise gotest makes is stated once in [`specs/`](specs/README.md), each scenario bound to a test; this reference explains and links there.

All assertions accept `*gotest.T`, `*testing.T`, `*gotest.B` and `*gotest.F` as first argument, and report a failure at the line that failed in each.

Run the CLI as `go tool gotest <args>`, declared once with `go get -tool github.com/mvrahden/go-test/cmd/gotest@<the version go.mod already requires>` (unversioned `go get -tool` upgrades the pin). Never a global `go install` binary: it drifts from go.mod; a CLI built with an older Go than the module's go line, or pinned below the oldest runtime it generates for, refuses to run and names the version the module selects.

## Rules

These override any default instincts from stdlib `testing` or other Go test frameworks.

**Never call `t.T().Helper()` when using gotest assertions.**
gotest automatically resolves assertion failures to the correct call site in your test code.
Calling `t.T().Helper()` is counterproductive — it removes useful location information from failure output.
Without it, failures show both where the assertion was called (inside the helper) and where the helper was called (in the test).
This applies to all gotest assertion functions (`gotest.Equal`, `gotest.True`, etc.).
If you pass `t.T()` to third-party code that uses Go's standard `t.Errorf`, Go's `t.Helper()` convention applies as normal.
This holds in a module whose path has no dot and under `-trimpath` too ([Failure locations](specs/assertions/spec.md#requirement-failure-locations)).

**Never call `t.T().Cleanup()` inside suite methods.**
Use `BeforeAll`/`AfterAll` for suite-level resources and `BeforeEach`/`AfterEach` for per-test resources.
The generated harness manages cleanup ordering (LIFO).
Calling `t.T().Cleanup()` inside suite methods breaks the predictable lifecycle.

**Never call `t.T().Fatalf()` in reusable test helpers.**
Use gotest assertions instead.
Inside `Eventually`/`Consistently` the callback gets a `*gotest.R`, which has no `T()` method, so a helper that needs `t.T()` cannot be used there at all.
Under a benchmark every hook's `t.T()` is nil, under a fuzz target `BeforeAll`'s and `AfterAll`'s (its `BeforeEach`/`AfterEach` run per input with a real `*testing.T`); the generator refuses a hook whose own body calls it there unchecked, but cannot see a helper the hook calls ([Hooks that call t.T() where it is nil](specs/cli/spec.md#requirement-hooks-that-call-tt-where-it-is-nil)).
Helpers built on gotest assertions work in every position.

**Never use `t.T().Skip()` for environment gating.**
Use `SuiteGuard() string` instead.
SuiteGuard runs before the suite's fixtures, config resolution, `BeforeAll`, and every test method — the suite's own setup never executes.
A shared fixture the suite binds is the exception: the runner starts it anyway, and its setup failure fails the suite before the guard is evaluated ([A guard does not hold back a shared fixture](specs/fixtures/spec.md#requirement-a-guard-does-not-hold-back-a-shared-fixture)).
A guard reads the environment, not fixture fields: they are still nil when it runs.
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
All except `Eventually`, `Consistently` and `MatchSnapshot` (whose trailing arguments name the snapshot) accept optional trailing `msgAndArgs ...any` for custom error messages.

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
gotest.Same[V any](t, expected, actual *V)           // pointer identity (==), never structure
gotest.NotSame[V any](t, expected, actual *V)        // distinct pointers
```

`True(t, p == q)` on pointers is identity; the `assertion-simplify` rule rewrites it to `Same`/`NotSame`, never to `Equal`, whose `reflect.DeepEqual` compares structure. A struct or array holding a pointer, or two interfaces, is left alone for the same reason; `err == target` on errors names `ErrorIs` without a fix, since `errors.Is` also matches wrapped errors.

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
gotest.Empty(t, object any)                          // nil, or len == 0 (string, slice, map, chan, array; a pointer is followed); any other type fails the type guard
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

A section is keyed by the exact subtest name, testing's `#0` and `#01` suffixes included ([Snapshot sections](specs/assertions/spec.md#requirement-snapshot-sections)).
A baseline saved under the older key, without such a suffix, is still compared, and renamed to the exact key outside CI ([Baselines keyed by the release before exact names](specs/assertions/spec.md#requirement-baselines-keyed-by-the-release-before-exact-names)).
`B` and `F` carry `Name()`, so benchmark and fuzz snapshots are keyed by name too.
Under `-trimpath` the baselines are looked up in the package directory go test runs in, so a snapshot taken in a helper of another package lands beside the package under test.
A name that spans lines or spells a section header is refused ([Snapshot names](specs/assertions/spec.md#requirement-snapshot-names)).
`GOTEST_UPDATE_SNAPSHOTS` and `GOTEST_CI` are off when unset or set to `0`, `f`, `F`, `false`, `FALSE` or `False`, and on for any other value; with updates off a stale baseline fails and stays untouched ([Snapshot updates switched off](specs/assertions/spec.md#requirement-snapshot-updates-switched-off)).

### Typing

Assertions take the types they are given at face value and say what they cannot work with:

- `Regexp` and `Contains` treat a named string type as a string ([Named string types](specs/assertions/spec.md#requirement-named-string-types)); a `Contains` needle the haystack cannot hold fails naming both types and, for a number of another numeric type, the type to convert it to ([A needle of the wrong type](specs/assertions/spec.md#requirement-a-needle-of-the-wrong-type)), and a nil needle is the nil element or key of a slice or map whose elements or keys can be nil ([A nil needle](specs/assertions/spec.md#requirement-a-nil-needle)).
- `InDelta` fails on a NaN delta and holds equal infinities within any delta ([InDelta at the edges of float64](specs/assertions/spec.md#requirement-indelta-at-the-edges-of-float64)), and measures integers exactly ([InDelta on integers](specs/assertions/spec.md#requirement-indelta-on-integers)).
- A first extra argument of any string kind formats the rest; any other puts every extra argument in the message, and `ErrorAs` names the type it expected ([Failure messages](specs/assertions/spec.md#requirement-failure-messages)).
- `JSONEq` compares numbers by value ([JSON numbers](specs/assertions/spec.md#requirement-json-numbers)) and refuses anything after the value ([One JSON value](specs/assertions/spec.md#requirement-one-json-value)), and `Equal` on values that do not fit one short line marks the lines that differ in a diff of their field-by-field renderings, each hunk led by the path of its first change such as `at [1].Name:` ([Mismatch diffs](specs/assertions/spec.md#requirement-mismatch-diffs), [Renderings](specs/assertions/spec.md#requirement-renderings), [Mismatches without a diff](specs/assertions/spec.md#requirement-mismatches-without-a-diff)); it names different dynamic types, diffs values that render alike by their raw fields or says where the rendering hides the difference, and never shows an address ([Values that look alike](specs/assertions/spec.md#requirement-values-that-look-alike)).

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

A poll that started before the deadline is judged by its result, however many ticks it takes; one still running `waitFor` past the deadline fails as still running ([Polls and their deadline](specs/assertions/spec.md#requirement-polls-and-their-deadline)), and a `waitFor` or `tick` that is not positive fails before the first poll ([Polling arguments](specs/assertions/spec.md#requirement-polling-arguments)).

**Critical constraint:** Inside the callback, `poll` is a `*gotest.R`, not a `*gotest.T`.
`poll` has no `T()` method. All code in the callback must use gotest assertions only.
A helper that takes `*gotest.T` cannot be called with `poll`.
This is by design — the recorder intercepts failures without aborting, enabling retry semantics.

A panic in the callback fails the test and is reported with the stack from where it
happened. The callback runs on its own goroutine, so the panic is carried back to the
test's goroutine before being re-raised — `AfterEach`, `AfterAll` and fixture teardown all
still run. A poll abandoned as still running keeps running, since Go cannot stop it; a
panic it raises later is written to stderr with its stack, not reported as a failure
([Abandoned polls](specs/assertions/spec.md#requirement-abandoned-polls)).

## Goroutines Started by Tests

A panic on a goroutine the test starts is unrecoverable: Go terminates the process
without running any other goroutine's deferred work, so no `AfterEach`, no `AfterAll`
and no fixture teardown happens. No framework can guard a goroutine it did not create.

Use `gotest.Go` to start one instead. It captures the panic with the stack from where
it happened and re-raises it on the test's own goroutine, where it is reported like any
other test panic and every cleanup still runs.

For work that finishes on its own, wait where you want the panic to surface:

```go
wait := gotest.Go(t, func() { report = build(input) })
defer wait()
```

For a goroutine that runs until something stops it — a server, a poller — do **not**
wait inside the test. `gotest.Go` registers the wait as test cleanup, which runs after
`AfterEach` (for a goroutine started in `BeforeAll`, after `AfterAll`), so whatever stops
the goroutine has already run by the time anything waits for it:

```go
gotest.Go(t, func() { srv.Serve(l) })   // AfterEach closes the listener
```

A `defer wait()` here deadlocks instead: the test's own defers run before `AfterEach`,
so it waits for a goroutine nothing has stopped yet.
A fatal assertion on the goroutine fails the method at the wait, and a skip there skips it ([Goroutines started by gotest.Go](specs/lifecycle/spec.md#requirement-goroutines-started-by-gotestgo)).
In `BeforeAll` this holds for the hook's `t` and every `T` derived from it (`When`, `It`, `Each` rows, `gotest.NewTWithDeadline`), and in a hook taking `*testing.T` for `gotest.NewT(t)`, `NewTWithDeadline(t)` and `NewTWithContext(t, ctx)`: each goroutine started on one is waited for after `AfterAll`, before the fixtures are released, even when another fails ([Goroutines BeforeAll starts](specs/lifecycle/spec.md#requirement-goroutines-beforeall-starts)).
A subtest such a hook starts itself with `t.Run` is not one: a goroutine started on it is waited for when that subtest ends, before `AfterAll`.

## Suite Conventions

### Suite types

```go
type MyServiceTestSuite struct { /* state */ }           // test suite
```

- Name must end with `TestSuite`
- All methods must use pointer receivers
- Must have at least one `Test*`, `Benchmark*` or `Fuzz*` method. Without a `Test*` method there is no `Test<Suite>` function: a test run opens the suite only to replay its `Fuzz*` seeds

### Test methods

```go
func (s *MyTestSuite) TestCreate(t *gotest.T) {}         // standard test method
func (s *MyTestSuite) TestRead(t *gotest.T) {}
```

Methods can also accept `*testing.T` for stdlib compatibility.

A test that calls `os.Exit` fails, `os.Exit(0)` included, as under `go test`.

A method named `Test*Async` with a trailing `done func()` parameter completes when `done()` is called, not when it returns (`(t, done)`, or `(t, ctx, done)` with a returning `BeforeEach`); the suite's `Timeout` fails it if `done()` never is, located at the method's declaration — and only that: a method waiting for `done()` is not reported as still running. Prefer `Eventually` for anything that can be polled.

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

`FailFast` is judged per method: the methods not yet started are skipped, naming the first that failed, and a failure `BeforeAll` records without stopping skips nothing ([FailFast](specs/lifecycle/spec.md#requirement-failfast)).

A stop — Ctrl-C, gotest's `--timeout` — reaches the running method through `t.Context()` and the suite through `AfterAll`: the methods not yet started are skipped with the reason `run stopping`, `AfterAll` runs, then the fixtures are released ([Stopping a run](specs/lifecycle/spec.md#requirement-stopping-a-run)). A method body or `BeforeAll` that ignores its context for five seconds is cut short, its fixtures released without `AfterAll`, and the process ends by the signal; the five seconds bound only the body and `BeforeAll`, and `AfterEach`'s context does not end at the stop, so a slow `AfterEach` still runs to its end before `AfterAll` ([The grace a stopped method gets](specs/lifecycle/spec.md#requirement-the-grace-a-stopped-method-gets)). Benchmarks and fuzz seeds not started are skipped the same way ([Stopping benchmarks and fuzz seeds](specs/lifecycle/spec.md#requirement-stopping-benchmarks-and-fuzz-seeds)). A second stop signal to the suite process — under plain `go test`, a second Ctrl-C — skips what is left of the grace: the fixtures are released at once and the process ends by the signal; a third has the signal's default action ([A forced stop](specs/lifecycle/spec.md#requirement-a-forced-stop)).

**On failure and panic.**
`AfterAll` runs even when `BeforeAll` failed or panicked, so teardown may see partly
initialised state — guard it if `BeforeAll` can fail midway. Write `AfterAll` to release
whatever `BeforeAll` had reached, not to assume it finished.
The void `AfterEach` likewise runs even when `BeforeEach` panicked. The returning form
does not: with nothing returned there is no context to release, and the per-test state it
would clean up was never created.
A panic in a method ends the suite as it would under `go test`: the method's own
teardown and `AfterAll` run, and in a parallel suite the siblings still running are cut
short with it ([A panic among parallel methods](specs/lifecycle/spec.md#requirement-a-panic-among-parallel-methods)).

**Returning BeforeEach (required for `Parallel: true`):**

When `SuiteConfig` enables `Parallel` on a suite with per-test state, `BeforeEach` must return a context struct pointer to give each parallel test its own isolated state (field-less suites, or suites whose only fields are fixture pointers, may be parallel without one). The generator rejects a `BeforeEach` that returns nothing in a parallel suite, but it cannot see methods writing suite fields — with no `BeforeEach`, or beside a returning one: that compiles, and races. Test methods and `AfterEach` then accept this context as an additional parameter:

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

Fields: `Timeout` (per-test deadline), `SetupTimeout`, `FailFast` (stop on first failure), `Parallel`, `Exclusive` (dispatched strictly alone, after every non-exclusive suite; go test's `-p N` caps how many suite processes run at once — for suites whose verdicts measure wall-clock behavior or contend for resources).
Presets: `DefaultSuiteConfig()` (30s/30s), `IntegrationSuiteConfig()` (2m/5m).

A duration the returned config leaves at zero (omitted or explicit) behaves as if the
marker were absent: it gets the default. `gotest.NoDeadline` (any negative duration)
disables the deadline. Booleans are used as written. Composing onto a preset works too:

```go
func (s *MySuite) SuiteConfig() gotest.SuiteConfig {
    cfg := gotest.DefaultSuiteConfig()
    cfg.Parallel = true
    return cfg
}
```

A timeout does two things, and only one of them is defaulted.

It always bounds `t.Context()`. And it is a budget the phase is held to, enforced by
verdict — but only when **you** declare a positive duration in a `SuiteConfig`. `Timeout` cannot interrupt a
running test, because Go has no way to stop another goroutine, so a method that ignores
`t.Context()` and outlives the budget is failed instead. That failure is reported the
moment the deadline passes, while the method is still running, located at the line it is
running, and is also written unbuffered to stderr so it survives a test that never returns
at all (under `-json` it streams with its test instead).
A method's `Timeout` covers its `BeforeEach`, body and `AfterEach`, the fixtures' per-method
hooks included, and the context they get ends at it: a hook that overruns it fails the method,
located at the hook and naming it, and nothing after that hook starts but the `AfterEach` hooks
([A hook or body going wrong](specs/lifecycle/spec.md#requirement-a-hook-or-body-going-wrong)).
An async method's budget ends when `done()` is called; its `AfterEach`, its own and the
fixtures', is not budgeted.

A suite with no `SuiteConfig` method, or a duration its marker leaves at zero, is not held
to a budget: being failed against a number you did not choose is not a verdict you can act on. Bounding the process remains
`go test -timeout`'s job either way.

`SetupTimeout` works the same way for `BeforeAll`, `AfterAll` and the wait on the goroutines
`BeforeAll` started with `gotest.Go`, including for a setup that hangs and never returns. A declared fixture `Timeout` is judged when the hook
returns: a `BeforeAll` that succeeds only after it fails the fixture without a retry
(the work is done; a retry would build it twice), and its `AfterAll` still runs. A
fixture hook that never returns is not failed by its `Timeout`; the run's other deadlines end
it (go test's `-timeout`, `--setup-timeout`, or a shared fixture's teardown force-kill).

`Timeout` reaches `t.Context()` inside nested `When`, `It` and `Each` bodies too — a
nested behavior inherits the enclosing deadline, and its context is canceled when that
nested subtest ends. For a tighter budget in one place, use `gotest.NewTWithDeadline(t, d)`: on a `*gotest.T` it derives from the suite's context, deadline and stop included; on a `*testing.T` from the testing package's, which carries neither.

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
Runs before the suite's fixtures, config, `BeforeAll`, and any test method, so fixture fields are still nil inside it.
A shared fixture the suite binds is still started by the runner, which cannot evaluate the guard, and its setup failure fails the suite before the guard is evaluated ([A guard does not hold back a shared fixture](specs/fixtures/spec.md#requirement-a-guard-does-not-hold-back-a-shared-fixture)).
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

Setup that suites in the same package bind. Name must end with `Fixture`.
Each suite that binds one sets it up for itself and tears it down after its `AfterAll` (once more per `Fuzz*` method, whose seeds replay separately); for setup that must happen once per run, use a shared fixture.
A suite sets up only the fixtures it binds and their dependencies; a fixture it does not use never runs for it, so it cannot fail it.

```go
type DBFixture struct {
    Pool *pgxpool.Pool
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { /* start db, set f.Pool */ return nil }
func (f *DBFixture) AfterAll(ctx context.Context) error {
    if f.Pool != nil { // AfterAll also runs after a BeforeAll that failed early
        f.Pool.Close()
    }
    return nil
}
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
A fixture's `AfterEach` runs only when every fixture `BeforeEach` up to its own in dependency order returned without an error, a fixture with no `BeforeEach` included ([Fixture per-method hooks in dependency order](specs/fixtures/spec.md#requirement-fixture-per-method-hooks-in-dependency-order)).
So a `BeforeEach` that returns an error or panics gets no `AfterEach`: it releases what it acquired before returning.
An error a fixture's `BeforeEach` or `AfterEach` returns is reported at the hook's declaration ([Where a fixture hook failure is reported](specs/fixtures/spec.md#requirement-where-a-fixture-hook-failure-is-reported)).

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
That includes the release between two setup attempts: it fails the run even when a later
attempt comes up ([A failed release between attempts](specs/fixtures/spec.md#requirement-a-failed-release-between-attempts)).

A fixture whose `BeforeAll` ran gets its `AfterAll` however it came out — failed,
panicked or cancelled, and before each retry — so write it to release whatever
`BeforeAll` had reached; one whose setup ended before its `BeforeAll` ran gets none
([A package fixture's BeforeAll or AfterAll going wrong](specs/fixtures/spec.md#requirement-a-package-fixtures-beforeall-or-afterall-going-wrong), [A failed shared setup is released](specs/fixtures/spec.md#requirement-a-failed-shared-setup-is-released)).
The release before a retry runs like the final teardown: under the fixture's `Timeout`,
held to a declared one, and not cut short by a stop, after which no further attempt starts
([A stop during the release between attempts](specs/fixtures/spec.md#requirement-a-stop-during-the-release-between-attempts));
an `AfterAll` that never returns is ended by `--setup-timeout` or `go test -timeout`.
Likewise a shared fixture whose `Hydrate` ran gets its `Dehydrate` however it came out
([A failed Hydrate is dehydrated](specs/fixtures/spec.md#requirement-a-failed-hydrate-is-dehydrated)).

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
func (f *PostgresSharedFixture) Dehydrate(ctx context.Context) error {
    if f.Pool != nil { // Dehydrate also runs after a Hydrate that failed
        f.Pool.Close()
    }
    return nil
}

func (f *PostgresSharedFixture) connect(ctx context.Context) error {
    var err error
    f.Pool, err = pgxpool.New(ctx, f.ConnStr)
    return err
}
```

Fields assigned in `Hydrate` are automatically classified as local and excluded from serialization.
Only the runner provides that state: under plain `go test` a suite binding a shared fixture fails before any method runs ([Shared fixtures need the gotest runner](specs/fixtures/spec.md#requirement-shared-fixtures-need-the-gotest-runner)).
`gotest fuzz` and `gotest fuzz triage` start the shared fixtures a target's suite binds once for the session, and every fuzz worker hydrates them ([Fuzzing with shared fixtures](specs/fuzz/spec.md#requirement-fuzzing-with-shared-fixtures)).

Shared fixture configuration uses a method named `SharedFixtureConfig()` (not `FixtureConfig()`) but returns the same `gotest.FixtureConfig` type:

```go
func (f *PostgresSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
    return gotest.ContainerFixtureConfig() // 5m timeout, 1 retry
}
```

Suites reference shared fixtures via pointer fields (same as package fixtures).
Shared fixtures have no per-test hooks: the generator refuses one that declares `BeforeEach` or `AfterEach` ([A shared fixture with per-test hooks](specs/cli/spec.md#requirement-a-shared-fixture-with-per-test-hooks)). Put per-test work in a package fixture that binds the shared one, or in the suite's `BeforeEach`/`AfterEach`.
Shared fixtures must not live in `internal/` packages.
Shared fixtures are window-scheduled: each is resident exactly while a scheduled suite needs it — never started when no dispatched suite requires it, and released or started at the bulk→exclusive-tail barrier as the phases' needs differ.
`-skip` counts: a fixture no selected suite binds never starts ([Selection decides which shared fixtures start](specs/fixtures/spec.md#requirement-selection-decides-which-shared-fixtures-start)).

### Generic test suites (contract testing)

Generic suite structs define contract tests once; type aliases instantiate them for each concrete type:

```go
type IndexContractTestSuite[T Indexable] struct{}

func (s *IndexContractTestSuite[T]) SuiteConfig() gotest.SuiteConfig {
    cfg := gotest.DefaultSuiteConfig()
    cfg.Parallel = true
    return cfg
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
t.T() *testing.T                                    // access underlying testing.T (nil in every hook under a benchmark, in BeforeAll/AfterAll under a fuzz target)
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
Every row is its own subtest with its own verdict. A row that leaves early ends the method
there — Go's range-over-func cannot resume a loop whose body left through `FailNow` or
`Skipf` — so a row that should merely be passed over uses `continue` ([Each rows](specs/lifecycle/spec.md#requirement-each-rows)).

## Utilities

```go
gotest.Must[T any](val T, ok any) T  // panics if ok is error/false; returns val otherwise
                                      // useful in test setup: db := gotest.Must(sql.Open(...))
```
