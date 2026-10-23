# Canary

The canary checks gotest's verdicts from outside gotest. Every other test in
this repository reports its result through gotest's own runner, so a bug in
that runner could turn a failure into a pass without any test noticing. The
canary closes that gap: it builds the `gotest` binary, runs it over small
fixture packages written to fail in specific ways, and compares what
the binary reports with a checked-in expectation. It checks with plain Go
only, so nothing under test takes part in the verdict.

## Fixtures

Each directory under `testdata/` is one package with one suite:

| Fixture | Behavior it pins |
|---|---|
| `passing` | a green suite exits 0 and every method and behavior gets `pass`, named string types, equal infinities, `1` against `1.0` in `JSONEq` and a final poll at the deadline for `Eventually` and `Consistently` and a `NewTWithDeadline` T that keeps the suite's deadline included |
| `failing` | one failing call per assertion function, all 36, each reported as `fail`, and the failures whose message the canary reads back: a wrong-typed needle, a NaN delta, a non-string message argument, a named pattern, big JSON integers, a multi-line diff, a poll still running at its deadline, a tick that is not positive and a snapshot name that spans lines; a failing `Each` row fails itself and its method, the rows before it pass |
| `failnow` | a failed assertion halts the method; the statement after it never runs |
| `aftereach` | `AfterEach` runs even when the test failed |
| `lifecycle` | `BeforeAll`, the methods and `AfterAll` run in that order |
| `benching` | a capturing bench run reports a result for every declared benchmark, read from `bench --json` |
| `benchfailing` | a benchmark that fails, halts at `FailNow` or skips reports no result and turns the run red |
| `fuzzing` | seeds replay as subtests of their wrapper; a failing seed fails the wrapper |
| `asynchronous` | an async method passes when `done()` is called from another goroutine and fails at the deadline when it never is |
| `broken` | a package that does not compile exits 2 |
| `panicking` | a panic fails its method and the run is red |
| `fixtureteardown` | two suites bound to one package fixture, each in its own process with its own copy of the DAG: every process runs the fixture's `AfterAll`, and so does a bench run |
| `fixturefuzzing` | a fixture-bound suite's seeds replay while the fixture is still up |
| `teardownfailing` | a shared fixture whose `AfterAll` fails turns a green run red, and the `-json` stream carries the failure as a failed package |
| `fixturechain` | a shared fixture reached only through a parent package fixture is started and hydrated for the suite; under plain `go test` the suite fails naming the fixture and the runner before any method runs |
| `setupfailing` | a shared fixture that fails to come up fails only the suites that read it, in the `-json` stream and in a bench run |
| `teardownaftersetup` | a shared fixture whose `AfterAll` fails beside a sibling that never came up: the stream carries both failures |
| `guardskip` | a suite its guard skips sets up none of its fixtures: one that would fail leaves the run green |
| `snapshotting` | a value that differs from its committed baseline fails; one that matches passes, carriage returns at line ends included; unnamed `Each` rows and same-named subtests read sections of their own, also under `-trimpath`, and `GOTEST_UPDATE_SNAPSHOTS` spelled `0`, `false`, `FALSE` or `f` rewrites nothing; runs that may write use a staged copy |
| `snapshotlegacy` | baselines keyed the way the release before exact names keyed them are compared under the new key, left alone under `GOTEST_CI=1` and renamed otherwise; a section its exact key read serves no second key, and `GOTEST_CI=FALSE` is off |
| `exclusive` | an Exclusive suite starts only after every non-exclusive suite in the run ended |
| `latepolls` | a poll that starts before waitFor is judged by its result, for `Eventually` and `Consistently`, however many ticks it takes; a poll abandoned past its deadline that panics later is written to stderr |
| `helpers` | an assertion failing inside a helper names the helper's line and the caller's, under `-trimpath` in a module without a dot |
| `goroutines` | a `gotest.Go` goroutine started in `BeforeAll` is waited for after `AfterAll`; one that fails fatally fails its method at the wait, one that skips skips it there, and one that leaves through `runtime.Goexit` with no verdict fails it, saying so |
| `setupwaits` | `gotest.Go` goroutines BeforeAll starts on Ts derived from the hook's (`NewTWithDeadline`, `When`, `It`, `Each`), or on `gotest.NewT(t)` in a hook taking `*testing.T`, are waited for after `AfterAll`, as is one started while that wait runs; the wait is held to a declared `SetupTimeout`; when the last of several fails or panics, the suite fails and every other one is waited for before the fixture is released |
| `fuzzhooks` | a fuzz execution whose `BeforeEach` fails or panics still runs its `AfterEach` |
| `eachbreak` | a loop that breaks out of an `Each` table stops at the break: no later row runs and the method goes on after the loop |
| `eachskip` | a skipping `Each` row skips itself and the method, the rows before it pass; a row that skips or fails the method's own `T` ends the method with that verdict alone |
| `hookpanics` | a panic in `BeforeEach` is reported even when `AfterEach` then fails a fatal assertion, for a test method and a fuzz execution |
| `fuzzskiphook` | a fuzz execution whose `BeforeEach` skips is skipped, its body never runs and its `AfterEach` still runs |
| `hookoverrun` | a fixture `BeforeEach` or `AfterEach`, or a suite `BeforeEach`, that sleeps or waits on its context past the suite's `Timeout` fails the method, located at the hook and naming it; after a `BeforeEach` that overran, neither a later fixture's `BeforeEach`, the suite's `BeforeEach` nor the body runs, and a hook waiting on its context sees it end |
| `fixturehookfail` | a package fixture whose `BeforeEach` fails for one method runs no `AfterEach` for it, and the failure is reported at the hook's file |
| `fixtureafterfail` | a package fixture whose `AfterEach` returns an error is reported at the hook's file, not inside the runtime's |
| `fixtureorder` | a fixture with only an `AfterEach` runs it when it comes before a fixture whose `BeforeEach` fails, and not when it comes after |
| `fixtureelsewhere` | a failing fixture hook another package declares, a fixture's of that package or one promoted from its type, is located at its declaration: by base name, and by full path under `-fullpath` |
| `benchpanic` | a panicking benchmark turns the run red, the next benchmark still reports, and `AfterAll` runs before the fixture is released |
| `failfast` | a FailFast suite skips the methods after its first failure, each naming the method that stopped it |
| `failfastsoft` | a failure `BeforeAll` records without stopping does not stop a FailFast suite's methods |
| `failfastcleanup` | a parallel FailFast method that fails only in a cleanup, run one at a time, still stops the method that has not started |
| `parallelpanic` | a panic in a parallel method runs `AfterAll` and the fixture teardown while a sibling is mid-flight, and skips that sibling's `AfterEach` |
| `fixturesetupfail` | a package fixture whose `BeforeAll` fails still runs its `AfterAll`, and the suite that binds it never opens |
| `failingshared` | a shared fixture whose `BeforeAll` fails still runs its `AfterAll` |
| `packagerelease` | a package fixture whose `AfterAll` fails between its two setup attempts fails the run and reports that error, though the second attempt comes up |
| `sharedrelease` | the same for a shared fixture |
| `lastattempt` | a shared fixture that fails its last attempt fails the run, though the suite binding it would pass |
| `cancelbeforesetup` | a run stopped while a shared fixture's dependency sets up runs no `AfterAll` for the fixture, whose `BeforeAll` never ran, and the dependency's `AfterAll` |
| `releasestop` | a run stopped while a shared fixture's `AfterAll` releases a failed attempt lets that `AfterAll` finish, starts no further attempt and releases the fixture once |
| `releaseslow` | a package fixture whose `AfterAll` between its attempts outlives its declared `Timeout` fails the run, though the second attempt comes up |
| `hydratefail` | a shared fixture whose `Hydrate` fails partway still gets its `Dehydrate` |

`testdata/expected.txt` is the golden list. A `<fixture> exit <code>` line
gives the exit code; `<fixture> <action> <test>` lines list every `pass`,
`fail` or `skip` event the `-json` stream must carry, with `-` for the
package-level event.

Fixtures that must prove something ran (a teardown, an order) write a
marker file into the directory named by `GOTEST_CANARY_DIR`; the suite reads
it back after the run.

## Updating the golden list

Change `expected.txt` only when the behavior it pins has deliberately
changed. A failing check prints the rows it got and the rows it wanted side
by side; copy the new rows from there.
