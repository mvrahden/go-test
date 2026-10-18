# Mutation drill

The drill is a test of the tests. gotest runs its own test suites, so a bug
in gotest's core could hide a failure from gotest itself. The checks that
close that loophole (the census, the ring-0 suites and the canary) are only
worth trusting if they demonstrably catch such bugs. `make drill` plants one
deliberate bug at a time and requires those checks to fail.
The checks themselves are described in `ARCHITECTURE.md` under "Testing gotest", and its Glossary defines the terms used here.

## What a run does

For every `mutants/<name>.patch`, `drill.sh`:

1. copies the repository to a scratch directory;
2. applies the patch, which introduces one bug in a core component, with
   exact context and no fuzz (a line offset is accepted: the context still
   matches verbatim);
3. builds the copy and compiles the test binaries of the drill packages;
4. runs the ring-0 test packages, the canary and the conformance packages
   stating the patch's clauses in the copy with `gotest summary`;
5. expects that run to exit non-zero, and every `Expect:` line of the patch
   to match a line of its log.

The `gotest` that judges each run is built from the unmodified tree before
any patch is applied. The copy's test binaries still link the copy's mutated
runtime and assertion code, and the canary inside the copy builds and tests
the copy's own CLI. This is what lets a bug in the CLI's exit-code path be
caught: a mutated CLI cannot be trusted to report on itself.

The drill exits 0 only when every mutant was caught. It runs in CI
(`quality.yml`) on every push and pull request.

`bash tests/drill/drill.sh <name> ...` runs only the named mutants, which is
how a new patch is tried without waiting for the whole set.

## Mutants and the specs

A patch may carry `Clause: <WHEN> → <THEN>` lines above its diff, naming
the scenario outcomes in [`specs/`](../../specs/README.md) it falsifies: one
WHEN bullet and one of its THEN or AND bullets, joined by ` → `. The THEN is
written as in the spec. The WHEN is spelled as `gotest spec` renders a When
label: `when ` goes in front (`a flag is unknown` reads `when a flag is
unknown`); a WHEN whose first word is `when` in any case reads `when` and the
rest as written (`When the run stops` reads `when the run stops`); a WHEN whose
first word is another connective (`whenever`, `with`, `without`, `given`, `if`,
`unless`, `after`, `before`, `while`, `once`, `on`, `upon`, `as`, `for`,
`during`, `under`, in any case) is written as in the spec. `bash
tests/drill/drill.sh --list-unmutated` prints the clauses no patch names.
Their number may not exceed `tests/drill/unmutated-allow.txt`: a new clause
either gets a mutant or the budget is raised on purpose in the same change. A
conventions suite enforces both, and that every `Clause:` line names a clause
a scenario states; the listing and the suite share one reading
(`internal/speccatalog`), so they cannot disagree about what is unmutated.

A ring-0 twin in `tests/canary`, the same check in plain Go, is required
exactly when the mutant lives in what a test binary links (`pkg/gotest`,
`pkg/gotestruntime`, `internal/gotestgen`): a conformance suite asserts with
that code, so the drill then judges without the conformance suites, and the
`Expect:` lines match the twin's output. Any other clause is judged by its own
conformance suite: the drill adds the packages whose specs state the patch's
`Clause:` lines, and the `Expect:` line is that It's failure line as the judge
prints it. A `Clause:` line naming no clause of `specs/` fails the drill.

## Reading a failure

- `DRILL  <name>: SURVIVED` — the bug went unnoticed. A check has a hole;
  the log tail shows what the run reported. Fix the check, not the mutant.
- `DRILL  <name>: patch does not apply, refresh it` — the code the mutant
  targets has changed. Re-create the patch against the current source.
- `DRILL  <name>: mutant does not compile, refresh it` — the patch applies
  but breaks the build; the judge would exit 2 on that and hide whether any
  check fired. Re-create the patch.
- `DRILL  <name>: caught by the wrong check` — the run went red, but not
  through the check the patch's `Expect:` lines name. The mutant may have
  landed somewhere else, or the named check lost its hold; the unmatched
  expectations and the first failures follow.
- `DRILL  <name>: no Expect: line` — the patch does not name its check.
- `DRILL  <name>: Clause line names no clause of specs/` — a `Clause:` line
  quotes a sentence no scenario states; the unknown clauses follow.
- `DRILL  could not build the judge` — the unmodified tree does not compile.

## The mutants

| Patch | Bug it plants | Check that must catch it |
|---|---|---|
| `assert-always-passes` | `CheckEqual` reports success for every input | ring-0 assert suite: its raw checks see the wrong verdict string |
| `empty-always-true` | `IsEmpty` reports every value as empty | ring-0 predicates suite, and the canary's `Empty` row |
| `fail-is-noop` | `fail()` in `pkg/gotest` records nothing | canary: the fixtures that must fail stay green |
| `harness-drops-methods` | the shared suite-methods template omits the first method of each suite | census: declared methods have no verdict, exercised by the canary's passing fixture |
| `bench-harness-drops-methods` | the bench template omits the first benchmark of each suite | bench census, exercised by the canary's benchmark fixture |
| `fuzz-harness-drops-target` | the fuzz template omits the first fuzz target of each suite | canary: the fuzzing fixture's golden list (the fixture is red, so the census stands down) |
| `exit-code-zero` | `WorstExitCode` always returns 0 | canary: exit codes differ from the golden list |
| `tree-fail-is-pass` | `BuildTree` classifies `fail` events as `pass` | ring-0 tree suite (the canary reads raw events, not the tree) |
| `it-skips-body` | `gotest.T.It` opens the subtest and never runs its closure | canary: the behavior rows under every `It` vanish from the golden list |
| `run-failure-stays-on-stderr` | `bookRunFailure` books a run-level failure only into the captured stream, never the live `-json` one | canary: the `teardownfailing` fixture's `fail` row for the synthetic package vanishes from the golden list |
| `guard-after-fixtures` | the suite frame holds its fixtures before the guard runs | canary: the `guardskip` fixture's failing fixture turns a run its guard should leave green red, so its golden rows differ |
| `snapshot-always-matches` | `MatchSnapshot` never reports a differing value | canary: the `snapshotting` fixture's `fail` row vanishes from the golden list |
| `exclusive-runs-in-bulk` | the dispatcher never defers an Exclusive suite past the bulk | canary: the `exclusive` fixture's window stamps overlap |
| `teardown-error-dropped-after-setup-failure` | a sibling fixture's `AfterAll` failure is not booked once another fixture's setup failed | canary: the `teardownaftersetup` stream carries no word of the teardown failure |
| `bench-gate-passes-on-nothing` | a bench gate with no benchmark in both the baseline and the run passes | canary: the gate against a foreign baseline exits 0 instead of 1 |
| `bench-unmatched-dropped` | a bench comparison reports no benchmark as missing or added | canary: the report against a foreign baseline lists no missing benchmark |
| `bench-same-file-accepted` | `bench --save` and `--against` on one file compare the run against itself | canary: the same file for both is not refused with exit 2 |
| `bench-gate-judges-without-samples` | a bench gate judges a run with fewer than 4 samples a side | canary: a gate without `-count` is not refused with exit 2 |
| `bench-gate-refuses-enough-samples` | a bench gate refuses a run with exactly 4 samples a side | canary: the gate against a foreign baseline exits 2 instead of 1 |
| `bench-heuristic-mark-dropped` | a delta row judged by the ±20% rule of thumb is not marked as one | ring-0 render suite: the heuristic row lacks its mark |
| `bench-gate-zero-judges` | `--gate=0` arms a gate instead of disabling it | canary: a zero gate without `-count` is refused with exit 2 |
| `bench-verbose-behind-args` | the `-v` a bench run adds lands behind the user's `-args` | canary: a bench run with words after `-args` fails on the binary's flag parsing |
| `bool-false-reads-true` | a boolean flag given any explicit value, `=false` included, reads as true | cli conformance: `--ci=false` does not let a focused run pass |
| `foreign-flag-accepted` | a command takes a gotest flag another command owns | cli conformance: `clean --debug` is not refused with exit 2 |
| `flags-after-dashdash-dropped` | the flags after a bare `--` never reach the test binary | cli conformance: the binary's own check fails after `--` |
| `exec-command-left-in-build-flags` | `-exec <cmd>` lifts the command but leaves it among the build flags | cli conformance: a run under `-exec env` fails |
| `spaced-coverprofile-missed` | `-coverprofile <file>` is not read as the run's profile | cli conformance: summary prints no coverage for the spaced spelling |
| `spec-skips-stdlib-note` | spec runs without noting the stdlib tests it left to go test | cli conformance: spec's stderr lacks the stdlib note |
| `color-codes-into-pipe` | color follows only `--no-color` and `NO_COLOR`, never the terminal | cli conformance: summary into a pipe prints color codes |
| `json-summary-annotated` | summary writes GitHub annotations after a JSON document too | cli conformance: the JSON summary inside GitHub Actions carries `::error` |
| `summary-accepts-json-flag` | summary takes `-json` beside its terminal format | cli conformance: `summary -json` is not refused with exit 2 |
| `misspelled-command-runs` | a word one edit from a subcommand is not suggested | cli conformance: `sumary` runs instead of suggesting `summary` |
| `config-loaded-before-version` | `.gotest.yml` is loaded before version and help | cli conformance: version exits 2 under a config that does not parse |
| `scaffold-absolute-path-read-as-relative` | scaffold reads every file path relative to the working directory | cli conformance: an absolute path is not scaffolded beside the file |
| `stop-keeps-running-methods` | a stopped run still starts the methods that have not started | canary: the stopping fixture's later methods are not skipped, and they run after the parent or the stream reader is gone |
| `stop-releases-before-afterall` | a stop releases the fixtures before AfterAll runs | canary: the stopping fixture's markers are out of order |
| `fixture-aftereach-without-before` | a fixture's AfterEach runs for a method whose BeforeEach failed | canary: the hook-failing fixture's AfterEach shows up after the failed BeforeEach |
| `fixture-report-in-runtime` | a failing fixture hook is reported from the runtime's own file | canary: the hook-failing fixture's report does not name the hook's file |
| `timeout-watches-nothing` | a suite's `Timeout` holds nothing to a budget | canary: the slow-hook fixture's method passes instead of failing its budget |
| `async-wait-outlives-the-failed-goroutine` | an async wait goes on after the owner of done failed and no goroutine of the method is left | canary: the asyncfatal run takes the full 20s Timeout |
| `shared-fixture-runs-under-plain-go-test` | a suite binding a shared fixture runs under plain go test with empty state | canary: the staged fixture-chain suite's method runs instead of the suite naming the fixture it binds |
| `beforeall-goroutine-waited-before-afterall` | the waits of goroutines BeforeAll started run before AfterAll | canary: the server suite deadlocks until go test's alarm instead of passing |
| `go-wait-ignores-goexit` | a goroutine that left through FailNow counts as finished | canary: the method goes on after the wait on its failed goroutine |
| `fuzz-aftereach-skipped-when-beforeeach-leaves` | a fuzz execution registers AfterEach only once BeforeEach returned | canary: the AfterEach markers of the fuzz-only suites, the `fuzzskiphook` one included, are missing |
| `fixture-stdout-is-protocol` | a shared fixture's stdout is the protocol stream | canary: a setup that prints a fragment and a forged `_done` fails the run |
| `setup-wait-ignores-the-fixtures-budget` | the runner's wait for shared fixtures ignores the budgets they report | canary: a 2s-budget fixture taking 15s is waited for in full; fixtures conformance: a hanging 2s-budget fixture is given up only after 30s, and a container fixture taking 35s is given up at 30s |
| `skip-filter-ignored` | -skip plays no part in which suites a fixture starts for | canary: the fixture of a suite -skip names whole still starts |
| `skip-of-a-method-drops-the-suite` | a -skip naming one method drops the whole suite from the plan | canary: the suite's other method runs without its fixture |
| `fixture-failure-leaves-the-suite-unreported` | a suite whose shared fixture never came up gets no verdict | canary: the guarded suite's failed row is missing from the stream |
| `shared-fixture-failed-setup-not-released` | a shared fixture whose setup failed is not released | canary: the leaking fixture's AfterAll marker is missing |
| `release-leaves-the-group` | releasing a suite's process tree leaves its stragglers alive | canary: a sleep the test never waited for outlives the run |
| `method-panic-recovered` | the method loop swallows a method's panic | canary: the panicking method passes and its sibling runs to the end |
| `each-rows-share-the-method-verdict` | an Each row's assertions report on the method, not the row's subtest | canary: the failing row passes on its own and the row verdicts of the golden list are off |
| `regexp-named-pattern-is-empty` | a named string pattern compiles as the empty pattern | canary: the failing fixture's named pattern matches everything |
| `contains-type-guard-silent` | a needle a slice cannot hold is reported as merely absent | canary: no failure names the two types |
| `contains-named-kinds-refused` | a named string haystack is not a string | canary: the passing fixture's named-kind substring check fails |
| `indelta-nan-delta-passes` | a NaN delta admits every distance | canary: the failing fixture's NaN-delta assertion passes |
| `indelta-infinities-differ` | two equal infinities are not within any delta | canary: the passing fixture's infinity comparison fails |
| `message-drops-trailing-args` | a non-string first argument silences the arguments after it | canary: no failure carries the trailing argument |
| `erroras-type-unnamed` | ErrorAs reports a mismatch without the type it wanted | canary: no failure names the expected type |
| `jsoneq-numbers-as-float64` | JSONEq decodes numbers into float64 | canary: two integers a float64 cannot tell apart compare equal |
| `jsoneq-numbers-by-spelling` | JSONEq tells numbers apart by how they are written | canary: the passing fixture's 1 and 1.0 differ |
| `snapshot-keys-strip-twin-suffix` | a snapshot key drops testing's #01 and #0 | canary: twins and unnamed rows share one baseline and the second fails |
| `snapshot-update-switch-any-value` | GOTEST_UPDATE_SNAPSHOTS=0 rewrites baselines | canary: the stale baseline passes and the file changes |
| `snapshot-ext-from-file-only` | the _ext suffix comes from opening the caller's file alone | canary: under -trimpath the baselines are not found |
| `snapshot-dir-from-file-only` | the snapshot dir is always beside the caller's file path | canary: under -trimpath the baselines are not found |
| `snapshot-name-unchecked` | a multi-line snapshot name is written as it is | canary: the failing fixture's bad name passes |
| `poll-waited-out` | a poll that runs past the deadline is waited for | canary: the hanging polls of the failing fixture run their full 10s and no message names a running poll |
| `poll-final-skipped` | the deadline fails Eventually without a last poll | canary: the passing fixture's final poll never passes |
| `poll-args-unchecked` | a zero tick or waitFor is polled with | canary: the zero-tick method passes and no type guard is reported |
| `deadline-from-testing-context` | NewTWithDeadline derives from testing's context, not the suite's | canary: the tighter T outlives the suite's deadline |
| `diff-dropped` | a multi-line mismatch is reported without its diff | canary: no failure carries a unified hunk header |
| `callsite-ignores-the-main-module` | under -trimpath a dotless module path counts as the standard library | canary: a helper failure in such a module loses the caller's line |
| `bench-panic-skips-afterall` | the generated benchmark wrapper no longer recovers a panic | canary: the panicking benchmark fixture reports no result for the benchmark after it, and neither AfterAll runs |
| `failfast-drops-the-skipped` | a FailFast suite drops the methods it did not run instead of reporting them | canary: the FailFast fixture's later methods have no skip verdict |
| `failfast-trips-on-the-suite` | FailFast trips on a failure the suite recorded before its methods | canary: the soft-failing FailFast fixture's second method is not run |
| `fixture-failed-setup-not-released` | a fixture whose BeforeAll failed is not torn down | canary: the setup-failing fixture's AfterAll never runs |
| `bench-panic-taken-for-skip` | a benchmark's recovered panic skips the benchmark instead of failing it | canary: the panicking benchmark fixture's run exits 0 instead of 1 |
| `bench-stderr-dropped` | a plain bench run never prints a benchmark suite's stderr | lifecycle conformance: the panic and its stack are missing from stderr |
| `failfast-trips-before-a-failure` | a FailFast suite trips before any method failed, so its first method is skipped | canary: the FailFast fixture's run exits 0 and its first method has no fail verdict |
| `second-signal-taken-for-the-first` | every signal after the first counts as the same event, so a second Ctrl-C never abandons the teardown | lifecycle conformance: the second-SIGINT rows find the teardown finished and no word of the abandon |
| `second-signal-leaves-the-processes` | the CLI ends on a second signal without killing the process trees it runs | lifecycle conformance: the second-SIGINT rows find the suite process outliving the run |
| `interrupt-exit-keeps-the-verdict` | an interrupted run exits with the suites' worst code instead of 130 | lifecycle conformance and canary: the stopped run's exit code is not 130 |
| `stop-leaves-the-method-context` | a stop no longer ends a running method's context | canary: the stopping fixture's markers lack `context ended` |
| `each-break-ignored` | `Each` goes on to the next row after the loop body broke, so Go panics on the next yield | canary: the each-break fixture's method fails and never reaches the marker after the loop |
| `contains-nil-key-never-found` | `Contains` reports a nil key absent from every map, interface- and pointer-keyed ones included | ring-0 predicates suite: the nil-key rows of `IncludesElement` are not found |
| `contains-nil-key-read-as-zero-key` | an untyped nil needle is read as the zero key of any map, a string-keyed one included | ring-0 predicates suite: the string-keyed map is not refused |
| `jsoneq-trailing-data-accepted` | `JSONEq` ignores what follows the first JSON value, a stray `}` or `]` included | ring-0 predicates suite: `NormalizeJSON` accepts trailing data |
| `poll-cut-off-at-the-deadline` | a poll still running at waitFor counts as hung, with no tick of grace | canary: the `latepolls` fixture's late polls fail as still running |
| `watch-help-names-the-old-setup-default` | `help watch` gives `--setup-timeout`'s default as a flat 2m | cli conformance: the help watch row lacks the declared-budgets default |
| `hooks-calling-t-named-last-first` | the error for hooks calling `t.T()` lists them last first and is located at the last | cli conformance: the fuzz row's error names the hooks out of source order |
| `skip-of-the-test-function-drops-the-seeds` | a `-skip` naming a suite's Test function drops the suite, fuzz seeds included | fuzz conformance: the seed replay has no pass verdict |
| `skip-of-every-function-keeps-a-fuzz-suite` | a `-skip` naming every Test and Fuzz function never drops a suite with fuzz methods | fuzz conformance: the suite's shared fixture starts |
| `skip-kept-from-the-binary` | `-skip` is applied when suites are planned and never reaches the test binary | fuzz conformance: the skipped test method runs |
| `derived-t-drops-the-setup-waits` | a T from NewTWithDeadline loses the BeforeAll T's wait registry, so its gotest.Go waits run as cleanups before AfterAll | canary: the setupwaits fixture's derived-T suite does not pass |
| `async-wait-ends-at-any-failure` | an async wait ends at the first recorded failure, whoever's goroutine failed | canary: asynchelper's AfterEach runs before the owner calls done, and stopgrace's async AfterEach runs before the stop |
| `async-aftereach-unwatched` | after an async method's done, its AfterEach is no longer held to the Timeout | canary: asynchelper's overrunning AfterEach leaves the method passing |
| `stop-file-ignored` | the watcher no longer counts the stop file by itself, only a signal | canary: with the signals ignored, the stopping fixture's later methods are not skipped, verdicts are missing and AfterAll never runs |
| `plain-run-taken-for-the-runner` | a run without the runner's stop file falls back to a default one, so a plain run watches its stream | canary: the plainreader method's context ends when its reader goes |
| `setup-waits-not-deferred` | the waits BeforeAll registered run last first without defer, so a failing one skips the rest | canary: setupwaits' fixtures are released before the first goroutine ended |
| `parent-death-not-asked` | a suite process under the runner neither asks for its parent's death signal nor polls for its parent | canary: after the wrapper is killed the stopping fixture holds on, AfterAll never runs and the suite process lives |
| `death-signal-is-a-stop-signal` | the death signal is SIGTERM, so landing during a requested stop it counts as the second signal | runtime: the parent's death abandons the teardown a stop it asked for runs |
| `reader-loss-read-as-hangup` | a stream nobody reads is taken for one only on a hang-up, which a pipe's writer never gets | canary: after the runner and converter are killed the stopping fixture holds on, AfterAll never runs and the suite process lives |
| `stop-grace-past-5s` | a stopped body gets 15s, not 5s, before it is cut short | canary: the stopgrace run ends later and no message names 5s |
| `cut-short-releases-nothing` | a stop that cuts a body short ends the process without releasing the fixtures | canary: stopgrace's fixture AfterAll never runs |
| `stop-grace-bounds-the-hooks` | a stop waits for the hooks only as long as the body's grace | canary: stopgrace's fixture is released while the slow AfterEach still runs |
| `crashed-suite-named-by-the-timeout` | a test whose suite process crashed on its own stays counted as cut short | lifecycle conformance: the --timeout names the crashed test besides the hung one |
| `stop-signals-a-stranger` | the runner signals the pid a suite published even outside the suite's group | lifecycle conformance: the stranger gets SIGTERM and the suite's group no interrupt |
| `unsignalled-suite-killed` | a suite the runner cannot signal by pid is killed instead of interrupted | lifecycle conformance: the foreign-pid run's AfterAll never runs |
| `fixture-aftereach-reported-in-runtime` | a fixture AfterEach that returns an error is reported from the runtime's own file | canary: the AfterEach-failing fixture's report does not name the hook's file |
| `release-between-attempts-best-effort` | the AfterAll between two setup attempts is run bare and its error dropped | canary: a package and a shared fixture whose release between attempts fails leave the run green and say nothing of it |
| `last-attempt-failure-only-logged` | a setup whose last attempt fails is logged and then reported as a success | canary: a suite bound to a shared fixture that never comes up runs and the run goes green |
| `setup-budget-drops-the-release` | the setup budget a fixture reports leaves out the AfterAll between attempts | canary: a 2s-Timeout fixture whose attempts and release take 1.6s each is given up |
| `unattempted-shared-fixture-released` | the shared fixture process releases every started fixture, whether or not its BeforeAll ran | canary: a stopped run releases the fixture whose setup ended before its BeforeAll |
| `protocol-handed-to-started-processes` | the shared fixture process leaves the protocol descriptor and its variable to the processes a fixture starts | canary: the started process sees the variable and its write to descriptor 3 fails the setup |
| `orphan-exits-without-its-budgets` | a shared fixture process whose runner is gone exits at once instead of after its budgets | canary: the CLI killed while the fixture is up cuts the fixture's AfterAll short |
| `tail-dependent-before-its-dependency` | the runner keeps discovery's name order for shared fixtures, so a tail fixture listed before its tail-only dependency waits on a dependency the setup process never starts | fixtures conformance: the tail fixture and its dependency never come up and the run hangs |
| `late-start-waits-the-default` | a shared fixture started late, for the exclusive tail or a bench slot, is waited for a flat 2m instead of its declared budget | fixtures conformance: the tail and bench-slot waits outlast the fixture's 2s budget |
| `reported-teardown-budget-ignored` | the runner ignores the teardown budget on the setup process's `_done` line | fixtures conformance: the sibling's 8s AfterAll is force-killed at the 5s fallback |
| `late-start-outlives-the-run` | the wait for a late shared fixture start ignores the run's cancellation | fixtures conformance: the run's --timeout no longer ends the wait, and the fixture process is left behind |
| `late-teardown-budget-unreported` | the shared fixture process's acknowledgement of a start carries no teardown budget, so a fixture started late is released under the budget reported before it came up | canary: the `latebudget` fixture's hung AfterAll is force-killed after 30s instead of its 32s |
| `unready-fixture-served-empty` | a shared fixture that never came up is taken for one that did, and its state file is written without its state | fixtures conformance: the suite, the exclusive-tail suite and the benchmark reading a hung fixture run against an empty one |
| `given-up-setup-passes-the-run` | a shared fixture setup that is given up is not booked, and the suite reading it is recorded as passed | fixtures conformance: the run whose setup was given up beside a released sibling exits 0 |

## Adding a mutant

1. Pick a bug that one of the checks claims to catch, confined to one file.
2. Make the change in a scratch copy of the repository and save it as a
   unified diff with `a/` and `b/` path prefixes:
   `diff -u a/<path> b/<path> > tests/drill/mutants/<name>.patch`.
3. Run the copy's checks and read the log. Put one `Expect: <ERE>` line per
   signal of the named check above the diff, each matching a line of the
   log with colors stripped (a census verdict, a ring-0 suite's FAIL line,
   a canary golden mismatch), not merely a failure.
4. Run `make drill`. The new mutant must be reported as caught; add it to
   the table above with the check its `Expect:` lines name.

A patch that stops applying exactly, stops compiling, or is caught by
another check fails the drill, so a mutant cannot silently stop testing
anything.
