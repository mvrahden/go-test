# Mutation drill

The drill is a test of the tests. gotest runs its own test suites, so a bug
in gotest's core could hide a failure from gotest itself. The checks that
close that loophole (the census, the ring-0 suites and the canary) are only
worth trusting if they demonstrably catch such bugs. `make drill` plants one
deliberate bug at a time and requires those checks to fail.
The checks themselves are described in `ARCHITECTURE.md` under "Testing gotest", and its Glossary defines the terms used here.

## What a run does

`drill.sh` builds the orchestrator (`tests/drill/driver`, code in
`internal/drill`) from the unmodified tree and runs it. The drill requires
Linux 5.3 or later (pidfd support), Go, `patch` and `grep`.
The orchestrator and every supervisor are Linux subreapers, so processes that
fork, detach or leave a process group are still collected when a worker ends.
Neither the supervisor nor its verdict accounting uses the mutated framework.
Each supervisor runs in a session of its own and its worker in another, so a
mutant that signals a process group reaches neither the orchestrator nor
another worker. The orchestrator ignores a SIGINT or SIGTERM sent by a process
it started, and names it. A worker or supervisor that dies fails only its own
mutant: the orchestrator kills what it left behind, and the next mutant gets a
fresh worker.

The orchestrator snapshots the current repository, including uncommitted
changes, then builds the judge and reads the selection plan from that snapshot.
For each mutant, a worker:

1. restores the snapshot at its fixed source path, creates fresh temporary
   state and empties its gotest cache for child runs (see below);
2. applies the patch with exact context and no fuzz;
3. runs `go build ./...` to retain the whole-source compilation contract;
4. runs only its selected checks using the pristine judge's `-json` pipeline,
   which generates and compiles the actual harness once;
5. rejects any structured `build-fail` event, abnormal process termination or
   empty/malformed event stream, then renders the stream with the pristine
   `summary --input --render-only` command;
6. requires a nonzero judge result and every `Expect:` expression to match the
   rendered log, as a POSIX ERE (`grep -E`).

A worker has an independent wall-clock deadline covering copying, compilation,
execution and rendering. Expiry, missing results, planner failure, cancellation
and failed cleanup fail the drill; none count as a caught mutant. All selected
mutants must have exactly one terminal result. Every supervisor cleans its
remaining descendants before the worker directory is reused.

Only caches keyed on the current sources outlive a mutant: Go's build cache
(`GOCACHE`) and the worker's `cache/cli`, whose binaries the testkit files
under a key from `go list -export` build IDs, the Go settings and the
checkout, and copies to a caller-private executable before running. The worker's `cache/children` (`GOTEST_TEST_CACHE_DIR`)
holds the overlay and binary-cache entries of gotest's own child runs. Go does
not validate those, and a mutated CLI writes them, so each mutant starts with
it empty. The pristine judge's cache (`cache/judge`) is written only by the
pristine judge. Marker files, snapshots and other mutable test state are never
reused. The canary does not depend on shared fixture machinery to obtain its CLI.

Workers run without the host's `CI`, `GITHUB_ACTIONS` and `GOTEST_*`
settings, so a mutant gets the same verdict on a CI runner as on a laptop
(under `CI`, for one, a missing snapshot baseline fails by itself). The
bootstrap builds `test2json` into Go's build cache before any worker starts:
Go writes a tool there in place, and a run that resolves it meanwhile cannot
execute it.

The drill exits 0 only when every selected mutant was caught. CI splits it into
four shards, retains each shard's logs and phase timings even on failure, and
then checks with `--check-shards` that the shards split the mutants exactly.

`bash tests/drill/drill.sh <name> ...` selects named mutants; no names selects all.

| Variable | Meaning |
|---|---|
| `DRILL_JOBS` | Concurrent mutants in this invocation; defaults to half the available CPUs, at least one. It never changes which shard a mutant lands in. |
| `DRILL_SHARD=i/n` | Select one deterministic shard, with `1 <= i <= n`. |
| `DRILL_TIMEOUT` | Independent deadline per mutant in seconds, default 600. |
| `DRILL_LOGS` | Parent for a unique log directory per invocation; the actual path is printed. |
| `DRILL_TIMINGS` | Optional JSON file mapping mutant names to previous elapsed seconds. All shards must read the same file. |

Historical durations schedule long mutants first and greedily balance projected
shard wall time, accounting for the serial timing-sensitive tail: a shared
mutant counts half its duration, whatever the local CPU count. Unknown mutants
get an estimate of 30 seconds. The assignment reads only the mutant names,
their modes and `DRILL_TIMINGS`, so shards on runners with different CPUs
split one list exactly. Each shard writes the whole assignment to its
`selection-i-of-n.json`; `bash tests/drill/drill.sh --check-shards <dir>`
reads every selection file under `<dir>` and fails unless all shards computed
the same assignment, every shard wrote one selection, and each selected exactly
its part. The output `timings-i-of-n.json` files contain successful
observations; merge their objects to supply the next run's input.
Every result also records copy, patch, build, judge and render durations. A
`*.phase.json` file identifies the active phase if a worker times out.
Concurrent workers divide the available CPUs through `GOMAXPROCS`, Go's `-p`,
and the judge's compile and dispatch limits. Isolated mutants get the full budget.

The orchestrator's synthetic-process tests do not execute the drill:

```sh
go run ./cmd/gotest ./internal/drill
```

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
that code, so such a mutant is judged without the conformance suites, and
its `Expect:` lines match the twin's output. Any other clause is judged by its
own conformance suite, and the `Expect:` line is that It's failure line as the
judge prints it: `^FAIL .*conformance/<capability> ...`. A `Clause:` line
naming no clause of `specs/` fails the drill.

### What judges a mutant

A mutant runs only what its `Expect:` lines can match. A check left out of
the run cannot match them, so running less never passes for a catch: at
worst it turns one into a failure of the drill.

- An `Expect:` line anchored to a conformance package's `FAIL` line is
  judged by that package, under a `-run` selecting the suites and methods
  that state the patch's `Clause:` lines of that capability, read from
  `docs/design/catalog.json`; with no such `Clause:` line the whole package
  runs.
- Every other `Expect:` line is judged by the patch's `Judge:` lines, each
  selecting `Judge: <package dir> [<Suite>/<Method>[/<subtest>...] ...]`, the tests named by
  their declarations (`Judge: ./tests/canary CanaryTestSuite/TestLifecycleRunsInOrder`),
  with no test names selecting the whole package. Nested paths use literal subtest names
  from `catalog.json`, including underscores, and run separately to avoid regex
  cross-products. An explicit `Judge:` for a conformance package replaces that
  package's automatic method selection; unknown catalog paths are rejected.
- No check runs by default. The plan refuses an `Expect:` line that names a
  conformance package without starting `^FAIL .*conformance/<capability> `,
  and one that names another check while the patch has no `Judge:` line.
- A mutant whose runs reach a suite whose `SuiteConfig` sets `Exclusive`
  (the canary's timing suite) runs alone, after the others: its verdicts
  read the wall clock, so no other mutant may load the machine meanwhile.
  So does a patch with an `Alone: <why>` line, for a check that races the
  clock in a suite that does not declare `Exclusive`.

A conventions suite holds every patch to this reading
(`internal/speccatalog`): each `Judge:` line names a package directory and
tests it declares, and each `Clause:` line a clause `specs/` states.

## Reading a failure

Each mutant ends in one line, `drill  <name>: caught (<seconds>s)` or
`DRILL  <name>: <reason> (<seconds>s)`, and the run in
`drill  <judged>/<selected> mutants judged in <seconds>s`. The reasons:

- `SURVIVED, the guards did not catch it` — the bug went unnoticed. A check
  has a hole; its retained log shows what the run reported. Fix the check,
  not the mutant.
- `patch does not apply, refresh it` — the code the mutant targets has
  changed. Re-create the patch against the current source.
- `mutant does not compile, refresh it` — the patch applies but breaks
  `go build ./...`; the judge would exit 2 on that and hide whether any check
  fired. Re-create the patch.
- `generated judge harness does not compile, refresh it` — the judge's event
  stream carries a `build-fail` event: the patch breaks a test package.
- `caught by the wrong check; no line matches: <expression>` — the run went
  red, but not through the check the patch's `Expect:` lines name. Only the
  first `Expect:` expression that matches no line is named; the rest are not
  checked. The mutant may have landed somewhere else, or the named check lost
  its hold; the retained `<name>.log.plain` shows what the run printed.
- `no Expect: line` — the patch does not name its check.
- `judge emitted no events`, `judge emitted a non-JSON line`,
  `judge emitted an invalid event` — the judge's `-json` stream is empty or
  not a stream of events.
- `judge terminated abnormally (<code>)` — the judge exited other than 0, 1
  or 2, for example on a signal.
- `could not render judge events` — `summary --render-only` failed on the
  stream.
- `independent watchdog deadline exceeded` — the worker outlived
  `DRILL_TIMEOUT`; `<name>.log.phase.json` names the phase it was in.
- `worker cancelled`, `process cleanup failed: <error>`,
  `worker ended by <signal>`,
  `worker or supervisor exited without a valid result`,
  `worker produced no valid terminal result` — the worker or its supervisor
  ended without a verdict of its own.
- `DRILL  <name>: no terminal result` — a selected mutant got no verdict,
  because the drill was stopped from outside, a worker thread failed, or what
  a dead supervisor left behind could not be killed.
- `DRILL  stopped by SIGTERM from pid <pid> (<command>)` — something outside
  the drill stopped it, a `pkill -f` matching `gotest-drill` for one; the
  mutants not judged yet print `no terminal result`.
- `drill  ignored SIGTERM from pid <pid>, a process under judgment` — a
  mutant signalled the orchestrator; the drill goes on, and the mutant is
  judged by its own result.

A failure before any mutant runs prints one line and exits 1:

- `DRILL  could not build the drill driver` — `drill.sh` could not build the
  orchestrator; `go build` printed why.
- `DRILL  could not build gotest; see bootstrap.log` (or `clauses`,
  `test2json`) — the unmodified tree does not build.
- `DRILL  could not read judge plan; see plan.tsv and bootstrap.log` — a
  patch header cannot be judged as it stands. `plan.tsv` holds a
  `bad<TAB><name><TAB><reason>` line for each problem, such as
  `Clause line names no clause of specs/: <clause>` (a `Clause:` line quotes
  a sentence no scenario states), `./<dir> declares no <Suite>/<Method>` (a
  `Judge:` line names a test that is not there; a renamed check needs its
  `Judge:` lines renamed with it), `Alone: line gives no reason` or
  `no Expect: line, name the check that must catch it`.
- `DRILL  no mutant named <name>`, `DRILL  a mutant was selected more than once`,
  or a complaint about `DRILL_JOBS`, `DRILL_SHARD`, `DRILL_TIMEOUT` or
  `DRILL_TIMINGS` — the command line or environment is wrong.

`--check-shards` prints `drill  <n> shards agree on one assignment`, or
`DRILL  shards computed different assignments`,
`DRILL  expected one selection per shard 1..<n>, got <shards>`,
`DRILL  shard <i> selected other mutants than its assignment names`,
`DRILL  no selection files` or `DRILL  unreadable selection file` when the
shards of one run do not add up to every mutant exactly once.

## The mutants

| Patch | Bug it plants | Check that must catch it |
|---|---|---|
| `assert-always-passes` | `CheckEqual` reports success for every input | ring-0 assert suite: its raw checks see the wrong verdict string |
| `bench-panic-skips-afterall` | the generated benchmark wrapper no longer recovers a panic | canary: the panicking benchmark fixture reports no result for the benchmark after it, and neither AfterAll runs |
| `bench-panic-taken-for-skip` | a benchmark's recovered panic skips the benchmark instead of failing it | canary: the panicking benchmark fixture's run exits 0 instead of 1 |
| `bench-stderr-dropped` | a plain bench run never prints a benchmark suite's stderr | lifecycle conformance: the panic and its stack are missing from stderr |
| `body-starts-after-a-hook-overran` | a method's body starts after its `BeforeEach` overran the `Timeout` | canary: the body of the suite whose `BeforeEach` waits on its context runs |
| `callsite-drops-dotted-element-modules` | under `-trimpath` a module whose path has a dot only after its first element is not counted among the build's modules, so it reads as the standard library | conformance: a helper's failure in `corp/tools.v2` does not name both lines |
| `callsite-reads-escaped-root-package` | under `-trimpath` the `%2e` the linker writes for a dot in the last path element is not read back, so a module's root package reads as the standard library | conformance: a helper's failure in `corp/tools.v2` does not name both lines |
| `contains-lossy-number-hinted` | a number that the element type cannot hold exactly (`3.7` for an `[]int`, `300` for an `[]int8`) is told to convert, which would truncate or wrap it | ring-0 predicates suite |
| `contains-text-needle-unhinted` | a rune, byte or `[]byte` needle for a string is refused without naming the string type to convert it to | ring-0 predicates suite |
| `contains-wrapped-sign-hinted` | a number whose conversion wraps its sign both ways (`-1` for a `[]uint`) is told to convert | ring-0 predicates suite |
| `each-method-t-reported-as-a-goexit` | a row that skips or fails the method's own T is reported as leaving through Goexit without a verdict | canary: the eachskip method that skips in a row fails, saying its row left without a verdict |
| `empty-always-true` | `IsEmpty` reports every value as empty | ring-0 predicates suite, and the canary's `Empty` row |
| `fail-is-noop` | `fail()` in `pkg/gotest` records nothing | canary: the fixtures that must fail stay green |
| `failfast-drops-the-skipped` | a FailFast suite drops the methods it did not run instead of reporting them | canary: the FailFast fixture's later methods have no skip verdict |
| `failfast-reads-before-cleanup` | a parallel FailFast method's verdict is read before its cleanups ran | canary: both methods of the cleanup-failing FailFast fixture run under `-parallel=1` |
| `failfast-skip-located-in-runtime` | a FailFast skip is located in the runtime's `methods.go` | canary: the FailFast fixture's skips name a runtime file |
| `failfast-trips-before-a-failure` | a FailFast suite trips before any method failed, so its first method is skipped | canary: the FailFast fixture's run exits 0 and its first method has no fail verdict |
| `failfast-trips-on-the-suite` | FailFast trips on a failure the suite recorded before its methods | canary: the soft-failing FailFast fixture's second method is not run |
| `fixture-aftereach-only-never-runs` | a fixture with only an `AfterEach` never runs it | canary: the fixtureorder `AfterEach` ahead of the failed `BeforeEach` does not run |
| `fixture-aftereach-only-runs-out-of-turn` | a fixture with only an `AfterEach` runs it whether or not its turn came | canary: the fixtureorder `AfterEach` behind the failed `BeforeEach` runs |
| `fixture-aftereach-reported-in-runtime` | an error a fixture's `AfterEach` returns is located in the runtime's `methods.go` | canary: the fixtureafterfail failure names a runtime file, not the hook's |
| `fixture-aftereach-without-before` | a fixture's `AfterEach` runs for a method whose fixture `BeforeEach` failed | canary: the fixturehookfail markers show an `AfterEach` after the failed `BeforeEach` |
| `fixture-beforeeach-after-a-fixture-overran` | a fixture's `BeforeEach` starts after an earlier fixture's overran the `Timeout` | canary: the hookoverrun later fixture's `BeforeEach` runs |
| `fixture-beforeeach-context-unbounded` | a fixture's `BeforeEach` gets a context the `Timeout` never ends | canary: the waiting fixture gives up instead of seeing its context end |
| `fixture-hook-elsewhere-joined-onto-the-suite` | a hook declared in another directory is recorded by its base name, so `-fullpath` joins it onto the suite's directory | canary: the fixtureelsewhere `-fullpath` failure names a file that does not exist |
| `fixture-hook-elsewhere-unlocated` | a hook of a fixture another package declares is located through that package's own FileSet | canary: the fixtureelsewhere remote failure is unlocated |
| `fixture-report-in-runtime` | an error a fixture's `BeforeEach` returns is located in the runtime's `methods.go` | canary: the fixturehookfail failure names a runtime file, not the hook's |
| `fuzz-panic-discarded-by-aftereach` | a fuzz execution's AfterEach runs in the panic's unwinding, so its fatal assertion discards the panic | canary: the hookpanics fuzz execution's panic is never reported |
| `go-goexit-unexplained` | a goroutine that left through `runtime.Goexit` with no verdict fails the method without saying why | canary: no failure of the goroutines fixture names the Goexit |
| `go-skip-reported-as-a-failure` | a goroutine that skipped the test fails the method at the wait | canary: the goroutines fixture's skipping method fails |
| `harness-drops-methods` | the shared suite-methods template omits the first method of each suite | census: declared methods have no verdict, exercised by the canary's passing fixture |
| `bench-harness-drops-methods` | the bench template omits the first benchmark of each suite | bench census, exercised by the canary's benchmark fixture |
| `fuzz-harness-drops-target` | the fuzz template omits the first fuzz target of each suite | canary: the fuzzing fixture's golden list (the fixture is red, so the census stands down) |
| `exit-code-zero` | `WorstExitCode` always returns 0 | canary: exit codes differ from the golden list |
| `hex-row-start-named` | a hunk in a hex rendering names the start of its row, not the byte that differs | ring-0 equal suite |
| `hooks-calling-t-alias-unfollowed` | a hook's `t.T()` through a local alias of `t` (`tt := t`) is not seen | cli conformance: the alias row of the suites reaching `t.T()` other than by a plain call is accepted |
| `hooks-calling-t-fuzz-each-refused` | a fuzz suite's `BeforeEach` and `AfterEach` are refused for calling `t.T()`, which is real there | cli conformance: the fuzz suite whose per-input hooks call `t.T()` exits 2 instead of running |
| `hooks-calling-t-guard-ignored` | a `t.T()` checked for nil before use is refused like an unchecked one | cli conformance: the fuzz suite with guarded hooks exits 2 instead of running |
| `hooks-calling-t-named-last-first` | the error for hooks calling `t.T()` lists them last first and is located at the last | cli conformance: the fuzz row's error names the hooks out of source order |
| `late-goroutine-left-unwaited` | the drain waits once for what BeforeAll registered, so a goroutine started meanwhile is never waited for | canary: the setupwaits late suite passes and its panic is lost |
| `long-rendering-cut` | two long single-line renderings are shown cut at 160 bytes, hiding a difference past the cut | ring-0 equal suite |
| `long-unended-line-unmarked` | a long changed last line without a final newline is shown around its difference but without the `\ No newline at end of file` marker | ring-0 equal suite |
| `method-panic-discarded-by-aftereach` | `gotestruntime.AfterEach` runs AfterEach in the panic's unwinding, so its fatal assertion discards the panic | canary: the hookpanics test method's panic is never reported |
| `method-panic-recovered` | the method loop swallows a method's panic | canary: the panicking method passes and its sibling runs to the end |
| `overrun-names-no-phase` | an overrun's verdict does not name the hook that overran | canary: the slow fixture's verdict does not name its `BeforeEach` |
| `setup-waits-unwatched` | the wait on BeforeAll's goroutines is not held to the SetupTimeout | canary: the setupwaits slow suite gets no SetupTimeout verdict |
| `shared-fixture-each-hooks-accepted` | a shared fixture another package declares with a `BeforeEach` passes generation, and the hook never runs | cli conformance: the cross-package shared fixture row exits 0 instead of 2 |
| `suite-beforeeach-after-a-fixture-overran` | the suite's `BeforeEach` runs after a fixture's `BeforeEach` overran the `Timeout` | canary: the slow fixture's suite marks its `BeforeEach` |
| `testing-t-hook-misses-the-setup-waits` | `gotest.Go` finds the suite's waits only through the T's context, so a T wrapped around a `*testing.T` hook waits as a cleanup before AfterAll | canary: the setupwaits `*testing.T` suite does not pass |
| `timeout-watches-nothing` | a suite's `Timeout` holds nothing to a budget | canary: the slow-hook fixtures' methods pass instead of failing their budget |
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
| `beforeall-goroutine-waited-before-afterall` | the waits of goroutines BeforeAll started run before AfterAll | canary: the server suite deadlocks until go test's alarm instead of passing |
| `bench-skip-ignored` | a bench run plans and dispatches the benchmark suites `-skip` names | fixtures conformance: the fixture of the benchmark suite `-skip` names still starts |
| `bytes-one-per-line` | a byte slice that is not text renders one byte per line | ring-0 equal suite |
| `bytes-text-as-hex` | a byte slice of text renders as hex, so its lines are not diffed as text | ring-0 equal suite |
| `callsite-ignores-required-modules` | under `-trimpath` a dotless module the build requires is judged by import path alone, so it reads as the standard library | conformance: a behavior another dotless module declares does not name both lines |
| `callsite-ignores-the-main-module` | under `-trimpath` the main module's packages are judged by import path alone, so a dotless module reads as the standard library | canary: a helper's failure in a dotless module does not name both lines |
| `contains-convert-hint-dropped` | a number of another numeric type is refused without naming the type to convert it to | ring-0 predicates suite |
| `contains-interface-needle-unchecked` | any needle is taken for a slice of an interface type, one that does not implement it included | ring-0 predicates suite |
| `contains-named-kinds-refused` | `Contains` refuses a haystack of a named string type | canary: the passing fixture's typed-arguments method fails |
| `contains-nil-element-accepted` | a nil needle is looked up as the zero element of an `int` slice | ring-0 predicates suite |
| `contains-nil-element-never-found` | a nil needle is never found in a slice whose elements can be nil | ring-0 predicates suite |
| `contains-nil-substring-unguarded` | a nil needle for a string is reported absent, not refused, so `NotContains` passes | ring-0 predicates suite |
| `contains-nil-key-never-found` | a nil needle is never found in a map whose keys can be nil | ring-0 predicates suite |
| `contains-nil-key-read-as-zero-key` | a nil needle is looked up as the zero key of a map whose keys cannot be nil | ring-0 predicates suite |
| `contains-type-guard-silent` | a slice needle of a type the slice cannot hold is reported absent, not refused | canary: no failure message names both types |
| `contains-unhashable-key-looked-up` | a needle that is not comparable is looked up in a map, which panics | ring-0 predicates suite |
| `crashed-suite-named-by-the-timeout` | a test whose suite process crashed on its own stays counted as cut short | lifecycle conformance: the `--timeout` names the crashed test besides the hung one |
| `cut-difference-unnamed` | renderings cut before they differ do not name where the values differ | ring-0 equal suite |
| `deadline-from-testing-context` | NewTWithDeadline derives from testing's context, not the suite's | canary: the tighter T outlives the suite's deadline |
| `deleted-element-misaligned` | a deleted element is marked from its middle into the next one's | ring-0 equal suite |
| `depth-unnamed` | lines deeper than the indented levels do not name their depth | ring-0 format suite |
| `derived-t-drops-the-setup-waits` | `gotest.Go` looks the suite's waits up by the T's `testing.TB` alone, so a `When`, `It` or `Each` T of BeforeAll's waits as a cleanup before AfterAll | canary: the setupwaits fixture's derived-T suite does not pass |
| `diff-dropped` | `Equal` on multi-line values shows no diff | canary: no failure message carries the diff heading |
| `diff-file-headers-kept` | the diff keeps a file diff's `---`/`+++` headers above its hunks | ring-0 equal suite |
| `diff-final-newline-hidden` | the diff ends both sides in a newline, so a final newline only one side has shows no change | ring-0 equal suite |
| `each-break-ignored` | `Each` goes on to the next row after the loop body broke, so Go panics on the next yield | canary: the each-break fixture's method fails and never reaches the marker after the loop |
| `each-rows-share-the-method-verdict` | an Each row's assertions report on the method, not the row's subtest | canary: the failing and the skipping rows pass on their own, and the row verdicts of the golden list are off |
| `erroras-type-unnamed` | `ErrorAs` does not name the type it expected | canary: no failure message names `*fs.PathError` |
| `fixture-failure-leaves-the-suite-unreported` | a suite whose shared fixture never came up gets no verdict | fixtures conformance: the guarded suite gets no fail verdict |
| `flag-first-occurrence-wins` | a repeated `-run`, `-skip`, `-bench` or `-count` counts by its first value, where the flag package keeps the last | fixtures conformance: the suite the last `-skip` leaves runs without its fixture |
| `fuzz-aftereach-skipped-when-beforeeach-leaves` | a fuzz execution registers AfterEach only once BeforeEach returned | canary: the AfterEach markers of the fuzz-only suites, the `fuzzskiphook` one included, are missing |
| `go-wait-ignores-goexit` | a goroutine that left through FailNow counts as finished | canary: the method goes on after the wait on its failed goroutine |
| `hidden-difference-shown-twice` | values that render alike are printed twice instead of named as such | ring-0 equal suite |
| `hung-benchmark-named-as-its-suite` | a text bench run names the suite of a hung benchmark, not the benchmark | bench conformance: the `--timeout` names the suite instead of the hung benchmark |
| `hung-hook-suite-unnamed` | a suite that hangs before any method starts is never named | lifecycle conformance: the `--timeout` names the hung method but not the suite hung in its `BeforeAll` |
| `hunk-location-dropped` | a hunk does not name the path of its first change | ring-0 equal suite |
| `hunks-replacement-scattered` | a line replaced among repeated ones shows as an insertion and a deletion far apart | ring-0 equal suite |
| `indelta-difference-overflows` | `InDelta` subtracts integers in their own type, so the distance between the int64 extremes wraps to 1 | ring-0 compare suite |
| `indelta-infinities-differ` | `InDelta` fails two equal infinities | canary: the passing fixture's typed-arguments method fails |
| `indelta-integers-as-float64` | `InDelta` measures signed integers through float64 | ring-0 compare suite |
| `indelta-nan-delta-passes` | `InDelta` with a NaN delta passes | canary: the failing fixture's NaN-delta method passes |
| `jsoneq-exponent-zeros-kept` | `JSONEq` keeps a number's trailing zeros in its digits, so `100` and `1e2` differ | ring-0 predicates suite |
| `jsoneq-negative-zero-signed` | `JSONEq` keeps the sign of zero, so `-0` and `0` differ | ring-0 predicates suite |
| `jsoneq-numbers-as-float64` | `JSONEq` decodes numbers into float64 | canary: the failing fixture's big-integer method passes |
| `jsoneq-numbers-by-spelling` | `JSONEq` compares numbers by spelling | canary: the passing fixture's `1` against `1.0` fails |
| `jsoneq-trailing-data-accepted` | `JSONEq` ignores what follows the value | ring-0 predicates suite |
| `long-bytes-unpointed` | two long single-line byte slices of text show no offset of their first difference | ring-0 equal suite |
| `long-line-unpointed` | a long changed line is shown cut instead of around its first difference | ring-0 equal suite |
| `long-string-unpointed` | two long single-line strings that differ show no offset of their first difference | ring-0 equal suite |
| `map-key-ties-unordered` | map entries whose keys render alike keep the map's random order | ring-0 equal suite |
| `message-drops-trailing-args` | a message whose first argument is not a string keeps only that argument | canary: no failure message carries every argument |
| `message-named-format-ignored` | a message whose first argument is of a named string type is not used as its format | ring-0 format suite |
| `notequal-addresses` | `NotEqual` shows the shared value with `%#v`, addresses included | ring-0 equal suite |
| `raw-fields-unshown` | values whose GoString methods render alike are not shown field by field | ring-0 equal suite |
| `regexp-named-pattern-is-empty` | `Regexp` reads a pattern of a named string type as empty | canary: the failing fixture's named-pattern method passes |
| `render-nested-collapsed` | nested structs render on one line, so one changed field marks the whole line | ring-0 equal suite |
| `render-pointer-addresses` | pointers render as addresses, so equal elements behind distinct pointers are marked changed | ring-0 equal suite |
| `render-text-unsplit` | a string that spans lines renders on one line, so its text is not diffed by line | ring-0 equal suite |
| `render-unexported-skipped` | the rendering leaves unexported fields out, so a change to one is not marked | ring-0 equal suite |
| `run-cut-at-its-first-slash` | `-run` is matched by its text up to the first `/`, so a later alternative's suite is dropped | fixtures conformance: the suite the second alternative names does not run |
| `run-of-a-missing-test-function-replays-seeds` | a `-run` naming the Test function of a suite without test methods replays its seeds | fuzz conformance: the seeds replay and the shared fixture starts |
| `setup-waits-not-deferred` | the waits BeforeAll registered run last first without defer, so a failing one skips the rest | canary: setupwaits' fixtures are released before the first goroutine ended |
| `short-values-diffed` | values that each fit one short line are diffed instead of shown side by side | ring-0 equal suite |
| `skip-filter-ignored` | -skip plays no part in which suites a fixture starts for | fixtures conformance: the fixture of a suite -skip names whole still starts |
| `skip-kept-from-the-binary` | `-skip` is applied when suites are planned and never reaches the test binary | fuzz conformance: the skipped test method runs |
| `skip-last-alternative-decides` | the last `-skip` alternative matching a function decides instead of the first | fixtures conformance: the suite go test enters is dropped with its fixture |
| `skip-of-a-method-drops-the-suite` | a -skip naming one method drops the whole suite from the plan | fixtures conformance: the suite's other method runs without its fixture |
| `skip-of-every-function-keeps-a-fuzz-suite` | a `-skip` naming every Test and Fuzz function never drops a suite with fuzz methods | fuzz conformance: the suite's shared fixture starts |
| `skip-of-every-wrapper-keeps-a-testless-suite` | a `-skip` naming every fuzz wrapper never drops a suite without test methods, judged by a Test function it lacks | fuzz conformance: the suite's shared fixture starts |
| `skip-of-the-test-function-drops-the-seeds` | a `-skip` naming a suite's Test function drops the suite, fuzz seeds included | fuzz conformance: the seed replay has no pass verdict |
| `skip-pattern-left-unchecked` | a `-skip` pattern is not checked before the run, so one go test rejects runs nothing and passes | cli conformance: the pattern is not refused with exit 2 |
| `suite-ended-on-its-own-left-running` | what a suite process that ended on its own left open stays counted as running | lifecycle conformance: the `--timeout` names the crashed test besides the hung one |
| `sync-internals-shown` | values of package sync render their bookkeeping fields | ring-0 equal suite |
| `typed-long-strings-unpointed` | long single-line strings of different types show no offset of their first difference | ring-0 equal suite |
| `types-unnamed` | values of different dynamic types are not named as such | ring-0 equal suite |
| `unsigned-as-hex` | unsigned integers render in hex | ring-0 format suite |
| `waiting-test-named-by-the-timeout` | a parallel test still waiting for a slot counts as running | lifecycle conformance: the `--timeout` names the queued methods besides the one holding the slot |
| `watch-ends-before-fixture-aftereach` | a method's budget watch ends before its fixtures' `AfterEach` hooks run | canary: the method whose fixture `AfterEach` sleeps past the `Timeout` passes |
| `wide-caret-narrow` | the caret counts a wide character as one column | ring-0 equal suite |
| `poll-args-unchecked` | a zero tick or waitFor is polled with | canary: the failing fixture's zero-tick method passes and no message says the durations must be positive |
| `poll-cut-off-at-the-deadline` | a poll still running at waitFor counts as hung, with no grace | canary: the `latepolls` fixture's late polls fail as still running |
| `poll-final-skipped` | the deadline fails `Eventually` without a last poll | canary: the passing fixture's final poll never runs |
| `poll-waited-out` | a poll that runs past the deadline is waited for | canary: the failing fixture's hanging polls pass once their sleep ends, and no message names a poll still running past its deadline |
| `poll-grace-one-tick` | a poll in flight at the deadline gets one tick, not waitFor, before it counts as hung | canary: the `latepolls` fixture's polls of many ticks fail as still running |
| `poll-late-panic-swallowed` | a panic raised by a poll abandoned past its deadline is dropped | canary: the `latepolls` fixture reads no stderr line for it |
| `poll-final-skipped-consistently` | the deadline ends `Consistently` without a last poll | canary: the passing fixture's `Consistently` counts one poll, not two |
| `snapshot-dir-from-file-only` | the snapshot dir is always beside the caller's file path | canary: under `-trimpath` the `snapshotting` baselines are not found |
| `snapshot-ext-from-file-only` | the `_ext` suffix comes from opening the caller's file alone | canary: under `-trimpath` the `snapshotting` baselines are not found |
| `snapshot-keys-strip-twin-suffix` | a snapshot key drops testing's `#01` and `#0` | canary: twins and unnamed rows share one baseline and the second fails |
| `snapshot-name-unchecked` | a snapshot name that spans lines is written as it is | canary: the failing fixture's bad name passes |
| `snapshot-update-switch-any-value` | `GOTEST_UPDATE_SNAPSHOTS=0` rewrites baselines | canary: the stale baseline passes and the file changes |
| `switch-false-spellings-on` | `FALSE` and `f` switch `GOTEST_CI` and `GOTEST_UPDATE_SNAPSHOTS` on | canary: a stale baseline is rewritten and a missing one refused |
| `snapshot-legacy-ignored` | a baseline under the key the release before exact names used is not looked up | canary: the `snapshotlegacy` sections fail under `GOTEST_CI=1` |
| `snapshot-legacy-renamed-under-ci` | an old-keyed section is renamed under `GOTEST_CI` | canary: the `snapshotlegacy` baseline changes in a CI run |
| `snapshot-legacy-not-renamed` | an old-keyed section that matches keeps its old key | canary: the `snapshotlegacy` baseline is not migrated |
| `snapshot-legacy-claims-ignored` | an old-keyed section its exact key already read serves a second key | canary: the `snapshotlegacy` second twin compares against the first and fails |
| `snapshot-cr-runs-kept` | only CRLF is a line ending in a snapshot value | canary: the `snapshotting` fixture's carriage returns no longer match |
| `shared-fixture-runs-under-plain-go-test` | a suite binding a shared fixture runs under plain go test with empty state | canary: the staged fixture-chain suite's method runs instead of the suite naming the fixture it binds |
| `shared-fixture-failed-setup-not-released` | a shared fixture whose setup failed is not released | canary: the leaking fixture's AfterAll marker is missing |
| `fixture-failed-setup-not-released` | a fixture whose BeforeAll failed is not torn down | canary: the setup-failing fixture's AfterAll never runs |
| `release-between-attempts-best-effort` | the AfterAll between two setup attempts is run bare and its error dropped | canary: a package and a shared fixture whose release between attempts fails leave the run green and say nothing of it |
| `last-attempt-failure-only-logged` | a setup whose last attempt fails is logged and then reported as a success | canary: a suite bound to a shared fixture that never comes up runs and the run goes green |
| `unattempted-shared-fixture-released` | the shared fixture process releases every started fixture, whether or not its BeforeAll ran | canary: a stopped run releases the fixture whose setup ended before its BeforeAll |
| `release-cut-short-by-a-stop` | the release between two setup attempts runs under the setup's context, so a stop cancels it | canary: the stopped run's release between attempts sees its context cancelled |
| `attempt-after-a-stop-during-the-release` | a stop seen after the release between attempts still lets the next attempt start | canary: the stopped run's fixture starts a second attempt |
| `released-attempt-released-again` | an attempt released between retries stays marked for release, so the teardown releases it again | canary: the stopped run's fixture is released twice |
| `release-ignores-its-timeout` | the release between attempts is not held to the fixture's declared Timeout | canary: a package fixture whose release overruns its Timeout leaves the run green |
| `failed-hydrate-not-dehydrated` | a shared fixture whose Hydrate failed gets no Dehydrate | canary: the failed Hydrate's Dehydrate marker is missing |
| `fuzz-without-shared-fixtures` | `gotest fuzz` and its triage start no shared fixture for the targets that bind one | fuzz conformance: the fuzz workers and the triage re-run are refused for want of the fixture's state |
| `fuzz-target-runs-without-its-fixture` | a fuzz target whose shared fixture did not come up runs anyway, without its state | fuzz conformance: the target is refused by its own hold instead of never running |
| `stop-keeps-running-methods` | a stopped run still starts the methods that had not started | canary: the stopped fixture's later methods have no skip verdict |
| `stop-leaves-the-method-context` | a stop does not end a running method's context | canary: the stopped fixture's holding method never marks its context ended |
| `stop-releases-before-afterall` | a stop releases the fixtures before the suite's AfterAll runs | canary: the stopped fixture's markers show the release ahead of AfterAll |
| `stop-grace-past-5s` | a stopped method body that ignores its context gets 15s instead of 5s | canary: the stopgrace run does not say the body was cut short after 5s |
| `stop-grace-bounds-the-hooks` | the 5s grace a stopped body gets also bounds the AfterEach and AfterAll after it | canary: the stopgrace slow AfterEach is cut off before AfterAll and the release |
| `stop-grace-skips-beforeall` | a stop gives a `BeforeAll` that ignores its context no grace, so the run waits out the release bound | canary: the stopgrace hung `BeforeAll` is not cut short after 5s |
| `stop-grace-skips-late-bodies` | a body that starts after the stop gets no grace and is never cut short | canary: the stopgrace late body is not cut short 5s after it began |
| `stop-ends-the-aftereach-context` | a method's AfterEach gets the method's context, which the stop has ended | canary: the stopgrace AfterEach sees its context end instead of running to its end |
| `forced-release-gets-no-time` | a forced stop leaves the release none of the bound, so the fixtures are cut off mid-teardown | canary: the stopgrace slow release never ends |
| `second-signal-waits-out-the-grace` | a second stop signal is ignored while the grace runs | canary: the stopgrace fixture is not released at once on the second signal |
| `stop-runs-the-benchmarks-not-started` | a stopped benchmark run still starts the benchmarks not started | canary: the stopbench second benchmark runs |
| `stop-grace-skips-benchmarks` | a running benchmark body gets no grace and is never cut short | canary: the stopbench benchmark that ignores the stop is not cut short after 5s |
| `stop-runs-the-seeds-not-started` | a stopped run still replays the fuzz seeds not started | canary: the stopbench second seed runs |
| `cut-short-releases-nothing` | a stop that cuts a body short ends the process without releasing the fixtures | canary: the stopgrace fixture's release marker is missing |
| `interrupt-exit-keeps-the-verdict` | an interrupted run exits with the suites' verdict instead of 130 | lifecycle conformance and canary: the stopped run's exit code is not 130 |
| `async-wait-outlives-the-failed-goroutine` | an async wait goes on after the owner of done failed and no goroutine of the method is left | canary: the asyncfatal method takes its full 20s budget |
| `async-wait-ends-at-any-failure` | an async wait ends at the first recorded failure, whoever's goroutine failed | canary: asynchelper's AfterEach runs before the owner calls done, and the stopgrace failed method's AfterEach comes before the stop |
| `async-stop-taken-for-a-missed-done` | a stop does not end an async wait, which runs on until the grace cuts it short | canary: the stopgrace waiting method gets no skip verdict |
| `async-aftereach-unwatched` | after an async method's done, its AfterEach is no longer held to the Timeout | canary: asynchelper's overrunning AfterEach leaves the method passing |
| `async-wait-ignores-who-holds-done` | a failed async wait ends while a pool job or a `time.AfterFunc` callback still holds done | canary: asynchelper's AfterEach runs before the AfterFunc owner calls done |
| `async-wait-labels-the-body-only` | the goroutines an async method's BeforeEach starts are not the method's | canary: asynchelper's AfterEach runs before BeforeEach's goroutine failed |
| `async-siblings-share-a-label` | every method carries one label, so a parallel sibling's goroutines count as the method's | canary: asynchelper's failed parallel method waits for its sibling's goroutine |
| `async-failed-wait-blamed-on-done` | a failed method whose wait lasts to the deadline is reported for a missed done | canary: asynchelper's hanging owner's output lacks what was left to wait for |
| `go-panic-unseen-by-an-async-wait` | a panic on a gotest.Go goroutine does not end an async wait | canary: asynchelper's panicking method takes its full 20s budget |

## Adding a mutant

1. Pick a bug that one of the checks claims to catch, confined to one file.
2. Make the change in a scratch copy of the repository and save it as a
   unified diff with `a/` and `b/` path prefixes:
   `diff -u a/<path> b/<path> > tests/drill/mutants/<name>.patch`.
3. Run the copy's checks and read the log. Put one `Expect: <ERE>` line per
   signal of the named check above the diff, each matching a line of the
   log with colors stripped (a census verdict, a ring-0 suite's FAIL line,
   a canary golden mismatch), not merely a failure.
4. Unless every `Expect:` line is a conformance `FAIL` line, add `Judge:`
   lines naming the tests that print them, so the mutant does not run the
   whole canary.
5. Run `make drill`. The new mutant must be reported as caught; add it to
   the table above with the check its `Expect:` lines name.

A patch that stops applying exactly, stops compiling, or is caught by
another check fails the drill, so a mutant cannot silently stop testing
anything.
