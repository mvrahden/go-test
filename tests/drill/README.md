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
