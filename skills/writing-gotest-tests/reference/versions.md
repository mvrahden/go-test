# Version differences

SKILL.md and the other references describe the newest release. Find the
installed version first (SKILL.md's version check), then read what differs
on it below. After an upgrade, the entries newer than the old pin are the
upgrade notes: each names the behavior it replaced.

## v1.25.x

The skill targets **v1.26.0+**. On **v1.25.x** everything in SKILL.md
still applies EXCEPT these, which fail there:

1. **The compose form (`config.md`)** — v1.25.x requires the
   `SuiteConfig` body to be a SINGLE return statement, so composing onto a
   preset is a generation error. Rule 4's literal
   `gotest.SuiteConfig{Parallel: true}` works; on v1.25.x a partial
   literal inherits the 30s defaults.
2. **Config literalism (rule 3)** — v1.25.x merges your marker over the
   defaults: `0` means "keep the default", `-1` disables, and
   `Parallel`/`FailFast` are one-way latches in that merge (once true,
   never reset to false).
4. **Filtering `Each` rows** — on v1.25.x, `-run` selecting individual
   `Each` entries (e.g. `-run 'TestX/#1'`) DEADLOCKS the test binary until
   `-test.timeout` kills it with no teardown, and so does any entry after
   a `-failfast` trip. Filter whole suites or methods only there.
5. **`gotest.Go` (rule 2)** — does not exist on v1.25.x.
6. **`SuiteGuard` (rule 10)** — ignored on a suite bound to a package
   fixture; v1.25.x evaluates it only on suites without one.
7. **`//nolint` placement** — only on the finding's line or the package
   clause; a comment on its own line above the finding suppresses nothing.

## v1.25.x exit codes

Exit codes on v1.25.x are weaker than they look — never treat a green
gotest exit alone as proof there: a package failing to compile mid-run, a
suite binary killed by a signal, and `spec --input` on a failing stream
all exit 0, `gotest lint` exits 0 on a package it could not analyze, and
fixture-bound packages mis-count teardown under `-skip 'Suite/case'` and
`-count>1` (fixtures can be released while tests still run). All of these
fail properly on v1.26.0+. Cosmetic-only on v1.25.x: no `[no suites]`
note, `migrate` leaves no TODO markers, `CI=false` still enables CI mode,
no expanded value diffs.

## Config durations

**Config durations (rule 3) changed twice:** from v1.30 a duration a
marker leaves at zero gets the default and `gotest.NoDeadline` (any
negative value) disables it; on v1.26–v1.29 a zero duration meant no
deadline; v1.25.x merged the marker over the defaults (see v1.25.x, item 2).
So on v1.26–v1.29 rule 4's `gotest.SuiteConfig{Parallel: true}` runs
without any deadline: compose onto `gotest.DefaultSuiteConfig()` there.

## v1.26

1. **`SuiteConfig.Retries` is gone** — a v1.25.x suite setting it stops
   compiling after the upgrade; delete the field.
2. **Timeout verdicts start** — a declared `Timeout` fails a test that
   outlives it ("exceeded its configured Timeout"); v1.25.x only bounded
   the context.

## v1.27

Rules and sections tagged **v1.27+** need v1.27.0 or newer; skip them on
v1.26.x and earlier. What v1.27 adds:

1. **`SuiteConfig.Exclusive`** — an unknown field on v1.26.x, so a suite
   declaring it does not compile there.
2. **The `shared-fixture-undeclared` lint rule** — absent on v1.26.x, and
   absent silently: its findings are simply never reported, so do not
   rely on the linter for fixture-window mistakes there.
3. **`gotest spec --static`** — renders the specification from source
   without running anything, so you can read what a package promises
   before (or instead of) executing it. It reports on stderr any method
   whose behaviors it could not enumerate — a `When`/`It` inside a
   condition or a loop, a non-literal description, an `Each` over a
   non-literal table — so treat "incomplete:" lines as a prompt to write
   the behavior literally if you want it visible in tooling.
4. **`spec --input --render-only`** — exits 0 on a failing stream, for
   callers that render results rather than gate on them; without it
   `--input` exits 1 whenever the stream carries a failure.
5. **A duration on every `spec` row** — each row carries the wall clock
   it occupied, never the sum of the rows beneath it. To find what is
   slow, read top-down and stop where the children stop explaining the
   parent; never add sibling durations up, because they overlap whenever
   anything ran in parallel and their total can exceed the clock that
   passed. A row larger than everything under it held that time itself:
   a subprocess started in a `When` body, or a `BeforeAll`. On v1.26.x
   only leaf rows carry a duration at all, so there the expensive test
   can be the one showing nothing.

## v1.28

1. **Go 1.27 support** — on Go 1.27, `gotest lint` below v1.28 cannot
   analyze anything with the x/tools version its release pins ("internal
   error: package … without types", exit 1); generation and runs still
   work. Upgrade before linting on Go 1.27.

## v1.29

Rules and sections tagged **v1.29+** need v1.29.0 or newer. What v1.29 adds:

1. **The spec speaks the `When` vocabulary (rule 8)** — every `When` label
   renders as `when <condition>` on every surface, and a label that opens
   with its own connective (`with`, `given`, `after`, `if`, …) renders as
   written. Below v1.29 a `When` label renders verbatim (on v1.25–v1.26
   with underscores shown as spaces, even in live runs), so a bare
   condition reads as a bare phrase there; still write the condition
   alone — it is what the framework's own suites always did, and the
   connective arrives with the upgrade instead of having to be edited out.
2. **The `behavior-wording` lint rule** — absent below v1.29, and absent
   silently (like `shared-fixture-undeclared` on v1.26.x): a `When("when
   …")` or `It("it …")` is never reported there, so apply rule 8 by hand.
3. **`spec --input` reads source** — a replayed stream renders the
   declared labels and vocabulary when the packages it names are loadable
   from the working directory. Below v1.29 a replay renders from subtest
   names alone (underscores become spaces, no connective), so a replay and
   a live run can spell one behavior two ways there; judge wording from a
   live `gotest spec` on those versions.
4. **Fuzz targets on suites (rule 9)** — `Fuzz*` methods taking
   `*gotest.F`, `gotest fuzz` with `triage`/`promote`, and the six `fuzz-*`
   lint rules; see `reference/fuzzing.md`. Below v1.29 write no fuzz
   targets at all: a stdlib `func FuzzX(*testing.F)` is invisible to gotest
   on every version, and the suite form does not compile there.
5. **Benchmark methods on suites** — `Benchmark*` methods taking
   `*gotest.B`, `gotest bench` with baselines and gates, the action's
   `bench` inputs, and the three `bench-*` lint rules; see
   `reference/cli.md`. On v1.27–v1.28 `*gotest.B` already exists but
   nothing runs `Benchmark*` suite methods: one compiles and silently
   never runs. Below v1.27 it does not compile. Write none below v1.29.

From **v1.29.1**:

6. **The census** — a green `spec`, `summary` or `-json` run exits 2 when
   a declared method produced no verdict (`reference/cli.md`). On v1.29.0
   and below nothing notices a test the harness dropped.

## v1.30

Rules and sections tagged **v1.30+** need v1.30.0 or newer. What v1.30 adds:

1. **Census bookings in the stream** — a declared method with no verdict
   is booked into `-json`, `spec`, `summary` and `watch -- -json` as a
   failed test, its suite and package failing with it; see
   `reference/cli.md`. On v1.29.1 the census speaks only on stderr and
   through exit 2; below v1.29.1 there is none.

2. **A zero duration means the default deadline** — on
   `SuiteConfig.Timeout`/`SetupTimeout` and `FixtureConfig.Timeout`, omitted
   and explicit zeros alike, so a partial literal is no longer a suite
   without deadlines. `gotest.NoDeadline` disables one. A suite that
   relied on `0` for no deadline (v1.26–v1.29) now has its context
   canceled at the default, so code that honors `t.Context()` fails there
   (a defaulted duration is never enforced as a verdict); write
   `gotest.NoDeadline` where no deadline was the intent.

3. **The `config-no-deadline` lint rule** — a literal negative duration on
   those fields; every negative disables the deadline, so the rule reports
   it (expressiveness tier) and `lint -fix` spells it `gotest.NoDeadline`.
   The rewrite keeps the meaning. Below v1.30 it is not reported.

4. **An expired `--timeout` exits 1 on every command and names the hung
   tests**; exit 130 always means Ctrl-C, and an interrupted compile no
   longer looks like a pass.

From **v1.30.2**:

5. **`Same`/`NotSame`** — pointer identity assertions; `assertion-simplify`
   rewrites `True(t, p == q)` to them. Below v1.30.2 they do not exist.
6. **`migrate` exits 1 when a run leaves a `TODO(gotest-migrate)` marker,
   and `--dry-run` previews** — below v1.30.2 it exits 0 with markers left
   and has no `--dry-run`.
7. **`.gotest.yml` rejects unknown keys** — a misspelled key is an error
   instead of being ignored.
8. **Every JSON document opens with a `version` field** — parsers of
   `discover`, `bench --json` and the like can read the producing release.

## v1.32

Rules and sections tagged **v1.32+** need v1.32.0 or newer. What v1.32 changes:

1. **Benchmark and fuzz suites get setup deadlines** — their `BeforeAll`/
   `AfterAll` contexts carry `SetupTimeout` (default 30s), and a declared
   one is enforced, as for any other suite. Below v1.32 those contexts had
   no deadline, so a slow setup there that honors its context (a container
   start) now needs a declared `SetupTimeout`, e.g.
   `IntegrationSuiteConfig()`. A benchmark's `AfterEach` now runs even
   when the benchmark fails or panics.
2. **A suite without `Test*` methods has no test run** — a benchmark-only
   suite's `BeforeAll` and fixtures run only when its benchmarks do, and a
   fuzz-only suite opens once per `Fuzz*` method to replay its seeds.
   Below v1.32 both also got an empty `Test<Suite>` that ran `BeforeAll`
   in every test run.
3. **A suite sets up only its own fixtures, after its guard** — the
   fixtures it binds and their dependencies, after its `SuiteGuard` and
   before its `BeforeAll` (`fixtures.md`). Below v1.32 a suite set up
   every package fixture of its package, unrelated ones included, before
   its `SuiteGuard`: a guard there sees fixture fields already set and
   cannot keep a fixture from starting. (`go tool gotest` runs each suite
   in its own process on every version, so a package fixture never ran
   once for several suites there.)
4. **Teardown runs when a run is stopped** — on Ctrl-C or an expired
   `--timeout`, `AfterAll` and fixture teardown still release what setup
   built.
5. **The runtime floor is v1.32.0** — a v1.32 CLI refuses a pin below it,
   which only a CLI not run through `go tool gotest` can meet.

## v1.33

Rules and sections tagged **v1.33+** need v1.33.0 or newer. What v1.33 changes:

1. **`t-escape` reports more `t.T()` calls** — `Context` and `Deadline`
   (the `*testing.T` context carries no suite `Timeout`; `-fix` writes
   `t.Context()` and `t.Context().Deadline()`), also inside a `*testing.T`
   helper a suite method hands `t.T()` to, plus `Error`, `Fail` and
   `Logf`. Expressiveness tier, so skippable. Below v1.33 none of these is
   reported, so code that linted clean there can report after the upgrade.
2. **`suite-lifecycle` flags `t.T().Run` in any function**, not only suite
   methods — integrity tier, so it cannot be skipped project-wide; turn
   the subtest into `t.It`/`t.When`.
3. **`lint -fix` applies composed and overlapping fixes in one run and
   reports what it could not fix** (exit 3), and `-diff` previews exactly
   that result as a patch; below v1.33 `-fix` stops after one pass and
   exits 0 silently, so re-run it and plain `lint` there, and `-diff`
   previews nothing usable.
4. **The runtime floor is v1.33.0** — a v1.33 CLI refuses a pin below
   v1.33.0, which only a CLI not run through `go tool gotest` can meet.
   Drift refusals name the version to pin instead of `@latest`.
5. **`os.Exit(0)` in a test fails the run**, as under `go test`; below
   v1.33 the plain run reported it as passed. A panicking or exiting test
   exits 1 in every mode; below v1.33 the plain run passed the binary's
   own status through (2 for a panic).
6. **Every matched package ends in a verdict**, in text and `-json`
   (`[no test files]`, `[no suites]`, `[no tests to run]`), and
   `spec`/`summary --input` exit 2 on a stream with no events, one cut
   short, or a build failure (unless `--render-only`); below v1.33 the
   first two exited 0 and a replayed build failure 1. A green pre-v1.33
   run over packages without suites wrote an empty stream, so replay
   those with `--render-only`.
7. **`migrate` type-checks its result** — exit 0 means every migrated
   package compiles; every line that does not, and every construct it
   cannot convert, carries a marker (exit 1, on every run until none is
   left); a pattern matching nothing or a package that did not compile
   before exits 2, as does a run whose files cannot all be written (it
   writes all or none). It converts a suite wherever the package declares
   its parts, exports an unexported suite's name, converts the `…f`
   assertion forms, skips generated files and checks only the host OS's
   builds. Below v1.33 hooks declared in another file than the suite
   silently never ran and a clean exit could leave code that does not
   compile.
8. **A shared fixture may omit `AfterAll`**; below v1.33 one without it
   passes generation and then fails to start.
9. **A Timeout or SetupTimeout overrun names the test's file and line**;
   below v1.33 it pointed at `asm_amd64.s`.
10. **`scaffold` on an interface** writes a contract suite whose
    `BeforeEach` picks the constructor per instantiated type (add a
    `case`; an alias without one fails with "no constructor for <type>");
    below v1.33 its `factory` field could not be set through a type alias
    and the suite nil-panicked.
11. **Patterns that match no package fail** — a run (any mode, bench and
    fuzz included) exits 2 and `lint` exits 1 when no package matched; a
    wildcard pattern that matches nothing beside others that match only
    warns (a path that does not exist still fails the run). Below v1.33
    such a run passed green having tested nothing.
13. **Assertion locations name the user's line everywhere** — in generic
    suite code inlined into the harness, on a plain `*testing.T`, in
    benchmarks and fuzz bodies, helpers included, and for `Skipf`;
    `go test -fullpath` is honoured. Below v1.33 these could point into
    gotest, reflect or `gotest_psuite_test.go`.
14. **Run output takes `go test`'s shape** — bare import paths and meta
    patterns (`fmt`, `std`, `all`) run instead of being dropped, standard
    library packages get `[no suites]`/`[no test files]`, a package whose
    suites are all `X_` or deselected reads `[no tests to run]`, and every
    package's `-json` events open with a `start` event.
15. **`-skip` passes through like `-run`**; v1.32 rejected it as an
    unknown flag. An empty pattern (`gotest "$PKG"` with `$PKG` unset) is
    the current directory, as for `go test`; `.go` file arguments exit 2
    (gotest takes packages, not files), and so does a bad test flag value
    or `-run`/`-skip` regexp — below v1.33 those ran and failed as a test.
    `gotest fuzz` refuses `-run`, `-skip` and `-bench`; pick the target
    with `--target`.
16. **`generate` and `clean` touch only the main module's packages**
    (go.work modules count) — `generate all` no longer writes harness
    files into dependencies. Bare `gotest clean` cleans the current
    package, like `generate`; use `gotest clean ./...` for the tree.

## v1.34

Rules and sections tagged **v1.34+** need v1.34.0 or newer, which in
practice means v1.35.0: **v1.34.0 is retracted** (its tag moved after the
module proxy recorded it, so the proxy and GitHub serve different code).
Upgrade a v1.34.0 pin as SKILL.md's version check says. What v1.34 changes:

1. **`gotest skill install` writes this skill** into `.agents/skills`
   (`--dir=.claude/skills` for Claude Code), as shipped with the pinned
   version, replacing an older copy. Below v1.34 there is no `skill`
   command: `gotest skill install` reads both words as package patterns
   and fails the build (`package skill is not in std`, exit 2).
2. **A panicking benchmark always fails** (`--- FAIL`, exit 1) — a panic
   in a benchmark method, its `BeforeEach`/`AfterEach`, or the suite's
   `BeforeAll`/`AfterAll` is recovered, printed with its stack, and fails
   that benchmark; the remaining benchmarks, `AfterAll` and the fixture
   teardown still run. Below v1.34 the panic went to Go's testing package,
   which ends the run (exit 2) but now and then loses the panic and
   reports `PASS` with exit 0 (a Go race, about 1 run in 200 under load):
   on an older pin, never take one green benchmark run as proof that the
   code under it does not panic.
3. **`NotContains` fails on a needle of another type** — `NotContains(t,
   []int64{1}, 3)` (an untyped `3` is an `int`) fails, naming the type to
   convert it to; below v1.34 it passed without comparing anything. Write
   `int64(3)`. The same holds for a map key, and for a string: a rune,
   byte, `[]byte` or nil needle fails, and `NotContains(t, s, 'x')` asks
   for `string('x')`. `Contains` with such a needle never matched before
   either. A number gets the hint only when it converts without loss:
   `3.7` for an `[]int` fails with none.
4. **Snapshot sections keep testing's `#` suffixes** — `t.Run("issue #42")`
   is keyed `issue_#42`, an unnamed `Each` row `#0`, a second same-named
   subtest `x#01`; below v1.34 the trailing `#<digits>` was stripped, so
   twins and unnamed rows shared one section. A baseline under the old key
   is still compared: under `--ci`/`GOTEST_CI` read-only, otherwise renamed
   to the new key once it matches. A section another test's exact key
   already holds is not reused, so the second twin (`x#01`) has no baseline
   until one run outside CI records it. Benchmark and fuzz snapshots, which
   below v1.34 all went to `unknown.snap` (`unknown_ext.snap`) as `_`, are
   not migrated: run once with `--update-snapshots` and delete the
   `unknown*.snap` files.
5. **`GOTEST_CI` and `GOTEST_UPDATE_SNAPSHOTS` read like `strconv.ParseBool`**
   — off when unset or `0`, `f`, `F`, `false`, `FALSE`, `False`; any other
   value is on. Below v1.34 only `0` and `false` were off, so
   `GOTEST_UPDATE_SNAPSHOTS=FALSE` rewrote baselines.
6. **`-skip` takes a suite out of shared-fixture planning** when go test
   would enter none of its functions: its `Test` function, if it has test
   methods, and every `Fuzz` wrapper; in `gotest bench`, its `Benchmark`
   wrapper
   ([selection](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-selection-follows-go-tests-matching)):
   a shared fixture no remaining suite binds never starts. As in go test,
   the first alternative matching a function decides, so
   `-skip 'TestA/One|TestA'` still enters `TestA`, and a `-skip` naming
   only a suite's `Test` function still replays its fuzz seeds
   ([skip](https://github.com/mvrahden/go-test/blob/main/specs/fuzz/spec.md#requirement-skipping-a-suites-functions)).
   Below v1.34 `-skip` played no part in planning, so the fixture started
   anyway.
7. **`-run`, `-skip` and `-bench` read as the testing package reads them** —
   a pattern is refused with exit 2 when one of its `|` alternatives or `/`
   levels does not compile on its own (`-skip '[]|x]'`), a repeated flag
   (`-run`, `-skip`, `-bench`, `-count`, `-parallel`, `-coverprofile`, `-p`)
   counts by its last value, and a `-run` alternative's suite is found
   behind an earlier alternative with a `/` (`-run 'TestA/x|TestB'`). Below
   v1.34 such a `-skip` ran nothing and passed, the first of a repeated flag
   counted, and `TestB` was dropped. A suite with only `Fuzz` methods has
   no `Test` function: `-run TestSuite` no longer replays its seeds.
8. **`gotest.Go` in `BeforeAll` is waited for after `AfterAll`**, on the
   hook's `t` and on any `T` derived from it (`When`, `It`, `Each`,
   `NewTWithDeadline`; `gotest.NewT(t)` in a hook taking `*testing.T`),
   every such goroutine even when one fails, held to a declared
   `SetupTimeout`
   ([goroutines](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-goroutines-beforeall-starts)).
   Below v1.34 the wait on any of them ran before `AfterAll`, so
   `gotest.Go(t, func() { <-s.stop })` in `BeforeAll`, stopped in
   `AfterAll`, deadlocked even on the hook's own `t`, and
   `NewTWithDeadline` took only a `*testing.T`.
9. **A fatal assertion on a goroutine started by `gotest.Go` fails the
   method at the wait**, a skip there skips it, and a `runtime.Goexit`
   with neither fails it, saying so
   ([goroutines](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-goroutines-started-by-gotestgo)).
   Below v1.34 the method went on after such a wait as if the goroutine
   had finished.
10. **A method's `Timeout` covers its hooks**: a `BeforeEach` or
    `AfterEach`, the suite's or a fixture's, that overruns it fails the
    method, located at the hook and naming it, and nothing after that
    hook starts but the `AfterEach` hooks; a fixture's `BeforeEach` gets
    the method's context, which ends at the `Timeout`
    ([hooks](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-hook-or-body-going-wrong)).
    **`FailFast` reports what it skipped**: each method after the first
    failed one is skipped with the reason `FailFast: <method>`, and a
    failure `BeforeAll` records without stopping no longer trips it
    ([FailFast](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-failfast)).
    Below v1.34 only the body was held to the `Timeout`, the methods
    `FailFast` left out of a sequential suite had no verdict, and a soft
    `BeforeAll` failure stopped every method after the first, in
    sequential suites.
11. **A fixture's `AfterEach` pairs with its `BeforeEach`**: it runs only
    when every fixture `BeforeEach` up to its own in dependency order
    returned without an error, and an error either hook returns is
    reported at the hook's declaration
    ([order](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-fixture-per-method-hooks-in-dependency-order),
    [where reported](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-where-a-fixture-hook-failure-is-reported)).
    Below v1.34 the `AfterEach` ran after a `BeforeEach` that failed or
    panicked too, so it had to tolerate what was never set up, and the
    error was located inside gotest's runtime. To migrate, make a
    `BeforeEach` that can fail midway release what it acquired before it
    returns the error: its `AfterEach` no longer runs for it.
12. **A shared fixture with `BeforeEach` or `AfterEach` is refused**:
    generation fails at the hook
    ([refusal](https://github.com/mvrahden/go-test/blob/main/specs/cli/spec.md#requirement-a-shared-fixture-with-per-test-hooks)).
    Below v1.34 one from another package passed generation and the hooks
    never ran. To migrate, move the per-test work into a package fixture
    that binds the shared one, or into the suite's hooks.
13. **A fixture whose `BeforeAll` ran is released however it came out**
    ([package](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-package-fixtures-beforeall-or-afterall-going-wrong),
    [shared](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-failed-shared-setup-is-released)):
    its `AfterAll` runs after a `BeforeAll` that failed, panicked or was
    cancelled, and before each retry, not after a setup that ended before
    `BeforeAll` ran. Below v1.34 a failed setup was never released and a
    retry built on top of the failed attempt. To migrate, write `AfterAll`
    to release whatever `BeforeAll` reached, not to assume it finished.
14. **A failed release between setup attempts fails the run**
    ([release](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-failed-release-between-attempts)).
15. **A suite binding a shared fixture fails under plain `go test`**,
    naming the fixture and `go tool gotest`, before any method runs
    ([runner](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-shared-fixtures-need-the-gotest-runner)).
    Below v1.34 it ran with the fixture's fields empty. `gotest fuzz` and
    `gotest fuzz triage` start the suite's shared fixtures and every fuzz
    worker hydrates them
    ([fuzz](https://github.com/mvrahden/go-test/blob/main/specs/fuzz/spec.md#requirement-fuzzing-with-shared-fixtures)).
16. **The release between setup attempts runs like the final teardown**
    ([stop](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-stop-during-the-release-between-attempts)):
    under the fixture's `Timeout`, failing the run when it overruns a
    declared one, and not cut short by a stop, after which no further
    attempt starts and `AfterAll` does not run again.
17. **A failed `Hydrate` is dehydrated**
    ([hydrate](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-failed-hydrate-is-dehydrated)):
    `Dehydrate` runs after a `Hydrate` that failed or panicked. To migrate,
    guard in `Dehydrate` what `Hydrate` may not have set.
18. **A stop lets the suite finish**
    ([stop](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-stopping-a-run)):
    on Ctrl-C or `--timeout` the methods not yet started are skipped with
    the reason `run stopping`, `AfterAll` runs, then the fixtures are
    released. A method body or `BeforeAll` that ignores its context is cut
    short after 5s, without `AfterAll`; the 5s bound only those, so a slow
    `AfterEach` runs to its end
    ([grace](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-the-grace-a-stopped-method-gets)).
    `AfterEach`'s context does not end at the stop; benchmarks and fuzz
    seeds not started are skipped too; a second stop signal releases the
    fixtures at once, and under gotest a second Ctrl-C sends the suites
    that signal, asks the shared fixtures to tear down and kills what
    still runs 2s later, or at a third Ctrl-C; on Windows it kills the
    suites at once
    ([second Ctrl-C](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-second-ctrl-c-to-gotest)).
    Under `-json` the stream carries every verdict of the stopped run, on
    Windows too, and ends with a failed run naming the suites the stop cut
    short
    ([stop file](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-stop-by-the-stop-file-alone)).
    Below v1.34 a stop released the fixtures and ended
    the process at once, without `AfterAll`, and only while fixtures were
    held. To migrate a `goleak.VerifyNone` inside a suite method, which now
    sees the stop watcher, pass it
    `goleak.IgnoreAnyFunction("github.com/mvrahden/go-test/pkg/gotestruntime.(*watcher).loop")`.
20. **Shared fixture setup waits for the declared budget** — without
    `--setup-timeout`, the runner waits as long as the fixtures'
    `SharedFixtureConfig()` declares, every attempt and every release
    between them counted, and 30s until the first reports
    ([the setup-budget requirement](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-shared-fixtures-declared-setup-budget)).
    Below v1.34 the wait is a flat 2m whatever the marker says, so a
    slower fixture needs `--setup-timeout` raised.
21. **A shared fixture started late comes up after its dependencies** —
    one started for the exclusive tail or a bench slot comes up after a
    dependency only that phase needs, is torn down before it, and is
    released under the teardown budget its declared `Timeout` gives
    ([dependencies](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-late-fixtures-dependencies),
    [teardown](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-late-fixtures-teardown-budget)).
    Below v1.34 one whose name sorted before that dependency's waited for
    a dependency not yet started, and its release got the default's
    budget whatever it declared. A stop or `--timeout` ends the wait for a
    late fixture.
22. **Shared fixtures can print freely** — the state travels on a
    descriptor of its own (on Windows, the process's own stdout, taken
    from fixture code before any of it runs), so what a setup prints to stdout, a line
    without a newline or a JSON-shaped log line included, never reaches
    it, and the state has no size limit
    ([state](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-shared-fixture-state-reaches-its-suites));
    on Unix, a process the setup starts gets no part of it and does not hold the
    run open
    ([started processes](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-process-a-shared-fixtures-setup-starts)).
    Below v1.34 the state shared the fixture process's stdout, so such
    output could break it, and a state line past 1 MiB failed the setup.
23. **A suite or shared fixture whose CLI died tears down by itself**
    ([suites](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-suite-whose-parent-is-gone),
    [shared fixtures](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-shared-fixture-process-whose-cli-is-killed)):
    after `kill -9`, an OOM kill or a crash of the CLI, a suite ends the
    holding method's context, starts no further method and runs
    `AfterAll`; a shared fixture process runs the `AfterAll` of every
    fixture whose `BeforeAll` ran, and on Linux exits with status 1 once
    its budgets have passed if a hook never returns. Below v1.34 nothing
    told them the CLI was gone: a suite could die without teardown, and
    the shared fixture process ran on, holding what its fixtures built.
    So does a suite whose stdout nobody reads any more under an `-exec`
    wrapper.
24. **No process a test started outlives the run**, even one it never
    waits for
    ([processes](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-no-process-outlives-the-run)).
    Below v1.34 such a process kept running after the run ended on Unix.
25. **`Eventually` and `Consistently` poll on a goroutine of their own** —
    a poll that started before `waitFor` is judged by its result, one
    still running `waitFor` past the deadline fails as still running
    ([polls](https://github.com/mvrahden/go-test/blob/main/specs/assertions/spec.md#requirement-polls-and-their-deadline)),
    and a `waitFor` or `tick` that is not positive fails before the first
    poll
    ([arguments](https://github.com/mvrahden/go-test/blob/main/specs/assertions/spec.md#requirement-polling-arguments)).
    Below v1.34 a poll that never returned hung the test, and a `tick`
    that is not positive panicked.
26. **Numbers compare by value** — `InDelta` fails on a NaN delta, holds
    equal infinities within any delta and measures integers exactly
    ([edges](https://github.com/mvrahden/go-test/blob/main/specs/assertions/spec.md#requirement-indelta-at-the-edges-of-float64),
    [integers](https://github.com/mvrahden/go-test/blob/main/specs/assertions/spec.md#requirement-indelta-on-integers)),
    and `JSONEq` keeps integers past float64's exact range apart
    ([JSON numbers](https://github.com/mvrahden/go-test/blob/main/specs/assertions/spec.md#requirement-json-numbers)).
    Below v1.34 both went through float64: a NaN delta passed, equal
    infinities failed and `9007199254740993` equalled `9007199254740992`.
    `Contains` now finds a nil key in a map keyed by an interface or
    pointer type
    ([nil needle](https://github.com/mvrahden/go-test/blob/main/specs/assertions/spec.md#requirement-a-nil-needle));
    below v1.34 a nil key was never found.
27. **A hook that calls `t.T()` where it is nil is refused** — every hook
    of a suite with a benchmark method, `BeforeAll` and `AfterAll` of one
    with a fuzz method; one error names every such hook in source order
    ([refusal](https://github.com/mvrahden/go-test/blob/main/specs/cli/spec.md#requirement-hooks-that-call-tt-where-it-is-nil)).
    Below v1.34 it passed generation and nil-panicked at run time; a call
    whose result is checked for nil is still accepted.
28. **The runtime floor is v1.34.0** — a v1.34 CLI refuses a pin below
    v1.34.0, which only a CLI not run through `go tool gotest` can meet.
    The v1.34 harness calls runtime functions v1.33.0 lacks, and the
    extension needs a v1.34 CLI.
29. **Flags read one way everywhere** — a boolean takes `--x`, `--x=true` or
    `--x=false`, the last one wins, and any other value exits 2; a value
    flag with no value, or one another command owns (`clean --debug`),
    exits 2. Below v1.34 `--ci=true` was silently ignored.
30. **go test flags keep go test's meaning** — `-p P` caps the suite
    processes and compiles, `-exec` wraps every suite binary, `-o` exits 2,
    `-run` takes a `Fuzz<Suite>_<Method>` name, `-parallel M` divides the
    budget instead of multiplying it (below v1.34: 2×GOMAXPROCS processes
    of M each), words after `-args` reach the binary, and `--min` and
    `summary` find `-coverprofile <file>` given as two words.
31. **Output follows its reader** — no color into a pipe whatever
    `--no-color` says, JSON documents carry no color or `::error` lines,
    and two output modes (`--spec -json`, `summary -json`) or an unknown
    `--format` exit 2. `version` and `help` print despite a broken
    `.gotest.yml`; `gotest sumary` and `gotest test` suggest the command
    meant (exit 2); every mode notes stdlib-only packages on stderr.
32. **A bench gate needs `-count=4`** (exit 2 below it) and fails when the
    baseline and the run share no benchmark; `--save` naming the
    `--against` file exits 2. Below v1.34 a gate judged any count and
    could compare a run against itself. Sub-benchmarks are keyed by path.
33. **An expired `--timeout` names only what was running** — not a test
    whose process had ended, nor one waiting for a `-parallel` slot; a hang
    in `BeforeAll` names the suite.
34. **`Equal` diffs field by field** — under `diff (-expected +actual):`,
    each hunk led by the path of its first change, with no addresses.
    `Regexp` and `Contains` take named string types (below v1.34 `Regexp`
    nil-panicked on one); a first message argument that is not a string
    keeps every argument in the message; helper lines are named under
    `-trimpath` in a module whose path has no dot.
35. **Snapshot names that span lines or spell a section header are
    refused**; a value with a trailing `\r\n` matches its baseline again;
    a panic in an abandoned poll goes to stderr.
36. **An `Each` row that skips ends the method as skipped** — below v1.34
    it failed it; a loop that breaks leaves the method running. A fuzz
    `BeforeEach` that fails or panics gets its `AfterEach`, and a panic is
    reported even when `AfterEach` then fails.
37. **A run sweeps what crashed runs left** — the `gotest-work-*` dirs of a
    dead owner in this boot and pid namespace go; unstamped ones after 7
    days, `--debug` ones are kept 7 days. A test binary briefly "text file
    busy" is retried.
38. **Each run writes its overlay into its own work dir** — nothing
    generated is cached, so concurrent runs sharing a cache never break
    each other's build
    ([generated files](https://github.com/mvrahden/go-test/blob/main/specs/cli/spec.md#requirement-generated-files-are-the-runs-own));
    `--no-cache` turns off only the test binary cache, and `--debug`
    prints the overlay inside the work dir. Below v1.34 overlays were
    shared under `~/.cache/gotest/overlays`; entries left there drain
    after 7 days unused.
39. **The binary cache is capped** — each run's start evicts the binaries
    of a checkout that no longer exists, those unused for 7 days, then the
    least recently used until the cache is under 5 GiB. A binary a run is linking or
    copying is never evicted, and a run whose lock was evicted while it
    waited links without the cache. Below v1.34 a key any run touched kept
    every binary.
40. **A context the default deadline ended is named** — one stderr line
    (with its test under `-json`) when the 30s default ends a method's,
    `BeforeAll`'s or `AfterAll`'s context in a suite that declares no
    `Timeout` or `SetupTimeout`
    ([default deadline](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-context-the-default-deadline-ended));
    it fails nothing.
41. **The GitHub action splits `packages`, `flags` and `go-test-flags` on
    every line** and exits 2 on quotes; below v1.34 only the first line
    counted.
42. **`GOTEST_SLOW_BUILD_THRESHOLD`** sets when a build is reported as
    still running (a Go duration, default `15s`).

## v1.35

Rules and sections tagged **v1.35+** need v1.35.0 or newer. What v1.35 changes:

1. **Test methods taking `done func()` are gone** — the generator refuses
   a `Test*` method whose last parameter is `done func()`, naming it; a
   method merely named `…Async` is an ordinary test. Rewrite one: close a
   channel from the callback, then wait for it in the method, asserting on
   the test goroutine:

   ```go
   done := make(chan struct{})
   go func() { defer close(done); /* the work that called done() */ }()
   select {
   case <-done:
   case <-t.Context().Done():
       gotest.Fail(t, "never finished")
   }
   ```

   A harness an older gotest wrote to disk still compiles; its first such
   method panics with the fix, which ends the suite's run, until
   `gotest generate` runs again.

2. **The runtime floor is v1.35.0 from v1.35.1** — a v1.35.1 CLI refuses
   a pin below v1.35.0, skipping the retracted v1.34.0; v1.35.0 still
   accepts a v1.34.0 pin. The extension from 1.35.1 needs a v1.35.0 CLI.

## v1.36

Rules and sections tagged **v1.36+** need v1.36.0 or newer. What v1.36 changes:

1. **Shared fixtures are released concurrently** — each after the fixtures
   holding a pointer field to it, as package fixtures already were. Two
   shared fixtures that share something without such a field (a Docker
   network, a port) used to be released one after another by accident; give
   the one that must go last a pointer field to the other.

2. **A shared fixture is released at its last use** — once no remaining
   planned suite declares it, not at the end of the run, and one only an
   exclusive suite needs starts just before that suite. A suite that reads
   a shared fixture it does not declare can find it gone; declare a pointer
   field for every fixture a suite touches.

3. **`SuiteConfig.ExclusiveGroup`** — a named group of suites run apart
   from every other suite and in parallel among themselves, the default
   group first, then each group by name; `--group` and `--skip-group`
   select groups (`config.md`). An unknown field before v1.36, so a suite
   setting it does not compile against an older runtime.

4. **`GOTEST_TIMEOUT_SCALE`** — set to a number above zero, it multiplies
   every suite and fixture budget, declared or default, deadline and verdict
   alike; set it on a CI runner slower than the machine the budgets were
   written on instead of raising them in code. Earlier versions ignore it.

## v1.36.1

Rules and sections tagged **v1.36.1+** need v1.36.1 or newer. What v1.36.1 changes:

1. **`fuzz promote` takes only the latest session's crashers** — the ones
   the latest `gotest fuzz` session of their target recorded, written with
   their exact values (a `-0` or a NaN payload included); a crasher that
   cannot be written exactly is refused and kept. Before v1.36.1 promote took
   every file in the corpus folder and deleted it, committed seeds included.

2. **`--ci` refuses focused benchmark and fuzz methods** — before v1.36.1 only
   a focused test method or suite failed the focus guard.

3. **`lint -fix` never flips a verdict** — `Nil`/`NotNil`/`Empty`/`NotEmpty`
   on an interface, float orderings and `True(errors.Is(err, target))` are
   reported without a fix, and a concrete error pointer becomes `Zero`/
   `NotZero`, not `NoError`/`Error`.

4. **A shared fixture's teardown is judged on its protocol** — a fixture
   that logs `X.AfterAll failed:` no longer fails a passing run.

5. **`bench --json` carries `failures`** — what a failed benchmark printed,
   a panic's message and stack included.

## v1.37

Rules and sections tagged **v1.37+** need v1.37.0 or newer. What v1.37 changes:

1. **Suites and fixtures are read from their own declarations** — a suite or
   fixture inside a grouped `type ( … )` block, an embedded suite or
   fixture, an embedded type with a method gotest reads, a field of suite
   type, a fixture field by value and two fields of one fixture type are
   each refused where they are written, naming the alternative
   ([declarations](https://github.com/mvrahden/go-test/blob/main/specs/discovery/spec.md#requirement-suite-and-fixture-declarations),
   [fields](https://github.com/mvrahden/go-test/blob/main/specs/discovery/spec.md#requirement-fixture-fields-and-case-names)).
   Before v1.37 an embedded fixture was bound and an embedded type's
   hooks were promoted; give each fixture a named pointer field and
   share tests through a generic contract suite.

2. **Focus is run-wide** — one `F_` suite or method of any kind, in any
   package or test package variant, skips every unfocused suite of the
   run, which then starts no fixture
   ([focus](https://github.com/mvrahden/go-test/blob/main/specs/discovery/spec.md#requirement-focus-across-a-run)).
   Before v1.37 focus held per package and test package variant.

3. **Aliases and generics work everywhere** — an alias of an alias, a
   method on a pointer to an alias, and an alias of a generic suite from
   another package or declared in the external test package all run
   ([other packages](https://github.com/mvrahden/go-test/blob/main/specs/discovery/spec.md#requirement-declarations-from-other-packages)).

4. **A suite declares a case** — a `*TestSuite` type declaring no
   `Test*`, `Benchmark*` or `Fuzz*` method, an excluded `X_` one counting,
   is refused at its declaration with exit 2, and `discover` reports its
   package broken; a testify suite is left to `go test`
   ([suites without a case](https://github.com/mvrahden/go-test/blob/main/specs/discovery/spec.md#requirement-suites-without-a-case)).
   `scaffold` refuses a type without an exported method or a file without
   an exported function, and `migrate` marks a testify suite without a test.
   Before v1.37 such a type was generated without a `Test` function, so it
   and its hooks never ran, and `discover` listed it as a suite without
   methods; name a helper type so it does not end in `TestSuite`.

5. **The harness imports under its own names** and every load reads past a
   generated file left on disk, so a package declaring `context` or
   `testing`, or a suite renamed since the last `gotest generate`, still
   runs
   ([generated harness](https://github.com/mvrahden/go-test/blob/main/specs/discovery/spec.md#requirement-the-generated-harness)).

6. **`migrate` converts base suites** — a testify suite embedding a base
   suite of the same test files migrates with it: the base becomes a helper
   held in a named field, `BaseTestSuite` renamed `BaseSuiteHelper`, and the
   child declares the hooks and tests it had through the base
   ([base suites](https://github.com/mvrahden/go-test/blob/main/specs/migrate/spec.md#requirement-base-suites)).
   Before v1.37 the files declaring both were left under a marker.

7. **`migrate` keeps testify's verdicts or marks why not** — a translation
   that would pass or fail where testify did not, or change what runs when,
   is left under a marker naming the reason and the gotest way
   ([kept verdicts](https://github.com/mvrahden/go-test/blob/main/specs/migrate/spec.md#requirement-kept-verdicts)),
   and the load never writes `go.mod` or `go.sum`, a vendored workspace
   included
   ([module files](https://github.com/mvrahden/go-test/blob/main/specs/migrate/spec.md#requirement-module-files)).
   Before v1.37 such a translation could turn a passing testify suite red,
   or a failing one green, with exit 0.

8. **`lint -fix` unfocuses files behind a build constraint** — the `focus`
   fix renames a suite in the files a build constraint leaves out too, and
   names, unapplied, a fix such a file would break
   ([the focus fix](https://github.com/mvrahden/go-test/blob/main/specs/lint/spec.md#requirement-the-focus-fix)).
