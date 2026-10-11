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
   preset is a generation error. Rule 4's literal `gotest.SuiteConfig{
   Parallel: true}` works; on v1.25.x a partial literal inherits the 30s
   defaults.
2. **Config literalism (rule 3)** — v1.25.x merges your marker over the
   defaults: `0` means "keep the default", `-1` disables, and
   `Parallel`/`FailFast` are one-way latches in that merge (once true,
   never reset to false).
3. **`Test*Async` (rule 1)** — the generated harness does not compile, and
   v1.25.x still exits 0 ("no test suites to run"); write a synchronous
   test with `Eventually` instead.
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
   analyze anything ("internal error: package … without types"; exit 0 on
   v1.25, 1 on v1.27); generation and runs still work. Upgrade before
   trusting a clean lint on Go 1.27.

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
3. **`lint -fix` reports what it could not fix** and exits 3; below v1.33
   it exits 0 silently, so run plain `lint` after it there.
4. **The runtime floor is v1.33.0** — a v1.33 CLI refuses a pin below
   v1.33.0, which only a CLI not run through `go tool gotest` can meet.
   Drift refusals name the version to pin instead of `@latest`.
5. **`os.Exit(0)` in a test fails the run**, as under `go test`; below
   v1.33 the plain run reported it as passed.
6. **`spec`/`summary --input` exit 2 on a stream with no events or one
   cut short** (unless `--render-only`); below v1.33 both exited 0.
7. **`migrate` leaves compilable code or a marker** — an assertion in a
   helper without `t` is marked, a renamed suite type is renamed in every
   file of the package, a still-used testify import is kept; a re-run
   exits 1 while markers remain; a pattern that does not exist exits 2.
8. **A shared fixture may omit `AfterAll`**; below v1.33 one without it
   passes generation and then fails to start.
9. **A Timeout or SetupTimeout overrun names the test's file and line**;
   below v1.33 it pointed at `asm_amd64.s`.
10. **`scaffold` on an interface** writes a contract suite whose
    `BeforeEach` picks the constructor per instantiated type (add a
    `case`); below v1.33 its `factory` field could not be set through a
    type alias and the suite nil-panicked.
