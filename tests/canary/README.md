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
| `passing` | a green suite exits 0 and every method and behavior gets `pass` |
| `failing` | one failing call per assertion function, all 36, each reported as `fail` |
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
| `fixturechain` | a shared fixture reached only through a parent package fixture is started and hydrated for the suite |
| `setupfailing` | a shared fixture that fails to come up fails only the suites that read it, in the `-json` stream and in a bench run |
| `teardownaftersetup` | a shared fixture whose `AfterAll` fails beside a sibling that never came up: the stream carries both failures |
| `guardskip` | a suite its guard skips sets up none of its fixtures: one that would fail leaves the run green |
| `snapshotting` | a value that differs from its committed baseline fails; one that matches passes |
| `exclusive` | an Exclusive suite starts only after every non-exclusive suite in the run ended |
| `eachbreak` | a loop that breaks out of an `Each` table stops at the break: no later row runs and the method goes on after the loop |
| `fixturehookfail` | a package fixture whose `BeforeEach` fails for one method runs no `AfterEach` for it, and the failure is reported at the hook's file |
| `hookoverrun` | a fixture `BeforeEach` that sleeps past the suite's `Timeout` fails the method, and the body never runs |
| `benchpanic` | a panicking benchmark turns the run red, the next benchmark still reports, and `AfterAll` runs before the fixture is released |
| `failfast` | a FailFast suite skips the methods after its first failure, each naming the method that stopped it |
| `failfastsoft` | a failure `BeforeAll` records without stopping does not stop a FailFast suite's methods |
| `fixturesetupfail` | a package fixture whose `BeforeAll` fails still runs its `AfterAll`, and the suite that binds it never opens |
| `goroutines` | a `gotest.Go` goroutine started in `BeforeAll` is waited for after `AfterAll`; one that fails fatally fails its method at the wait |
| `fuzzhooks` | a fuzz execution whose `BeforeEach` fails or panics still runs its `AfterEach` |
| `noisysetup` | a shared fixture that writes to stdout while coming up still hands its state to the suite whole |
| `setupbudget` | a shared fixture slower than its declared 2s `Timeout` fails the run at that budget, not when it finishes |
| `skipped` | a shared fixture bound only by a suite `-skip` names never starts; a `-skip` of one method still starts it |
| `guardedshared` | a guarded suite's shared fixture is still started, and when it fails the suite and the run fail |
| `failingshared` | a shared fixture whose `BeforeAll` fails still runs its `AfterAll` |
| `parallelpanic` | a panic in a parallel method runs `AfterAll` and the fixture teardown while a sibling is mid-flight, and skips that sibling's `AfterEach` |
| `helpers` | an assertion failing inside a helper names the helper's line and the caller's, under `-trimpath` in a module without a dot |
| `asyncfatal` | an async method whose done-owning goroutine fails fatally fails at once, without waiting for the `Timeout` |
| `stopping` | a run stopped while a method holds exits 130, skips the methods not started with the reason run stopping, ends the holding method's context and runs `AfterAll` before the fixture is released |
| `orphan` | a process a test started and never waited for does not outlive the run (Unix only) |

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
