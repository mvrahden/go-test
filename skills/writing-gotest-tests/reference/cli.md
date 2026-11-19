# CLI surface

Everything runs as `go tool gotest …` (see `setup.md`). Bare
invocation runs tests — there is NO `test` subcommand:

- `go tool gotest ./...` — run suites (never stdlib tests; those need
  `go test`). Passthrough flags work: `-v`, `-race`, `-count`, `-run`,
  `-json` (streams `go test -json` events incl. every subtest name).
  `-o` is refused with exit 2, and so are (v1.38+) `-c`, `-n`, `-overlay`,
  `-fuzz` (use `gotest fuzz`), and a profile flag (`-cpuprofile`, `-trace`, …) on a run
  that starts more than one suite process. `-run '^Test<Suite>$'` reaches
  only that suite's process and replays none of its fuzz seeds (v1.38+):
  name the `Fuzz<Suite>_<Method>` wrapper too, or drop the anchors.
  gotest has no result cache. Every run executes every suite it selects;
  it never prints `(cached)`.
  Outside CI mode a focused run notes on stderr that `F_` narrows it
  (v1.38+), and a text run fails with exit 2 when a suite's binary ran no
  tests (v1.38+).
- `go tool gotest spec ./...` — run + render the behavioral spec view.
  `--format terminal|md|json` (terminal is the default), `--output <file>`; `--input <file|->` re-renders a
  captured `go test -json` stream WITHOUT running (v1.29+: it still reads
  the declared labels and `When` vocabulary from the source it can reach
  from the working directory, so a replay renders exactly like the run;
  packages it cannot load render from the names alone). `--input` exits
  non-zero when the stream contains failures (same rule as
  `summary --input`), so replaying a saved stream in CI needs no pipefail
  gymnastics — the render step itself is the verdict; `--render-only`
  lifts that rule for pure renderers (exit 0 on a failing stream, 2 on an
  unreadable one). From v1.33 a stream with no test events, or one cut
  off before every package reported, exits 2 too. `--static` renders the
  spec from source without running anything — nodes carry no verdict, and
  incompletely enumerable methods are reported on stderr. `--declared`
  lays the verdicts over the specification the source declares, so a
  behavior that never ran shows as not run: with `--input` a saved
  stream's, and without it the run's own (v1.38+; below, `--input` was
  required);
  `--hide=pass` (or `fail`, `skip`, `none`, comma-separated) leaves rows
  out of the rendering and none out of the counts; `--match=<text>` and
  `--only=<import path>` keep part of the rendering, with the counts of
  what is kept; `--stamp` (with `--input`) writes the stream back instead
  of rendering it.
- `go tool gotest watch ./...` — rerun on change; supports `--spec`,
  `--bench` (re-run the benchmarks, with ns/op deltas) and
  `--debounce=<dur>` (default 200ms).
- `go tool gotest lint ./...` — the linter; exit 3 on findings (`-json`
  exits 0 and carries them in the document). `-fix` applies suggested
  fixes and formats the files it rewrites. From v1.33 it applies composed
  and overlapping fixes in one run, reports what is left, and writes
  nothing when the fixed code would not load or a file cannot be written
  (exit 1); `-diff` previews
  exactly what `-fix` would write as a patch for `git apply` run in the
  same directory (files outside it need `git apply --unsafe-paths`),
  writing nothing. `-tags` reaches the loader. Below v1.33 repeat `lint -fix` + plain `lint` until plain
  `lint` exits 0 (`-fix` stops after one pass and never shows what it
  cannot fix), and `-diff` is no preview: alone it only lists findings,
  and `-fix -diff` prints one pass with headers `git apply` rejects.
  `-rules` lists the rules (`-rules -json` as a document);
  `-disable-nolint` ignores `//nolint` comments.
  Integrity rules yield only to `//nolint:<rule>` (on the line, directly
  above it, or file-wide trailing the `package` line); others
  also to `.gotest.yml` `lint.skip` / `-skip-<rule>`. A plain finding
  line ends with its rule, `file:line:col: message (rule)` (v1.38+);
  `-nolint-allow` names other linters a `//nolint` may name (v1.38+, also
  `.gotest.yml` `lint.nolint-allow`); `--github=false` keeps the plain
  report inside GitHub Actions. golangci-lint can run the same analyzer
  as the module plugin `github.com/mvrahden/go-test/pkg/lint/golangci`
  (v1.38+), at the gotest version go.mod requires; `go doc
  github.com/mvrahden/go-test/pkg/lint` shows the setup.
- `go tool gotest discover ./...` — static suite metadata as JSON: suites,
  methods, direct suite→fixture edges, and (v1.27+) each method's declared
  `When`/`It` tree — `name` is the subtest segment `go test` will print,
  `display` the label as the spec renders it (v1.29+: spoken in its
  vocabulary, so `When("email is valid")` reads `when email is valid`),
  `kind` the call it came from. An `Each` over an inline literal table
  appears as rows;
  anything runtime-valued (a `When` behind a condition, a non-literal
  description or table) is missing and `behaviorsComplete: false` says
  so — never present an incomplete list as the whole specification.
- `go tool gotest scaffold ./pkg/path.TypeName` — generate a suite; an
  interface target generates a generic contract suite.
  `go tool gotest scaffold --fuzz ./pkg/path.FuncName` takes a
  single-parameter package-level function instead and scaffolds a
  `Fuzz<Func>` method for it, with round-trip and crash-safety skeletons.
  Own-module targets
  only (it writes into the target package's directory).
- `go tool gotest migrate ./...` — testify/suite conversion (see
  `migration.md` for its scope); `--dry-run` prints the diff of every edit
  instead of writing it.
- `go tool gotest refactor toggle-focus <file> <Suite[.Method]>` — toggles
  the `F_` focus prefix only (`X_` is a manual edit).
- `go tool gotest skill install [--dir=<skills-dir>]` (**v1.34+**) —
  rewrites this skill as shipped with the pinned version (default
  `.agents/skills`; Claude Code reads `.claude/skills`). Rerun after
  upgrading the pin.
- `go tool gotest summary ./...` — run and print a failure-first summary
  for CI; `--input <file|->` renders a saved stream, `--coverage=<path>`
  names the coverage profile, `--badge=<path>` writes a coverage badge SVG.
- Also: `prepare`, `generate`, `clean`, `version`, `help <cmd>`.

Flags shared by run modes: `--ci` (or env `GOTEST_CI`/`CI`; `0`, `f`, `F`,
`false`, `FALSE` and `False` are falsy, v1.34+; below, `0` and `false`) — enables the focus guard (committed `F_` prefixes fail
the run) AND makes a missing `MatchSnapshot` baseline fail instead of
being created. `--update-snapshots` rewrites baselines — in CI too, so
never pass it there. `--spec` renders the spec view
instead of default output; `--no-color` strips ANSI codes from it.
`--group=<name>` runs only the suites of that `ExclusiveGroup` and
`--skip-group=<name>` leaves them out (both repeatable).
`--compile-parallel=<n>` bounds concurrent compilations (default NumCPU,
halved under `-race`). `--github` emits annotations and the step summary
(on by default in GitHub Actions). `--events-file=<path>`, with `-json` or
in `summary`, also writes the event stream to that file. `--timeout <dur>` is the global pipeline
deadline (default 15m) and `--setup-timeout <dur>` a flat shared-fixture
setup budget (default: what the fixtures declare, v1.34+) — `0` disables either; `--min <pct>` gates
coverage, `--no-cache` skips the test binary cache and forces a fresh link (`gotest clean --cache`, **v1.38+**, empties it), `--debug` keeps the run's work dir and overlay.
`--control=stdin` (**v1.37+**, every command that stops on Ctrl-C) reads
`stop` and `force` lines on stdin, a first and a second Ctrl-C; the input's
end is a stop. For a program driving gotest that cannot signal it (Windows);
without the flag stdin is never read, and `--input=-` refuses it.

## Benchmarks — `gotest bench` (v1.29+)

`go tool gotest bench ./...` runs `Benchmark*` suite methods (signature
`func (s *X) BenchmarkParse(b *gotest.B)`, or stdlib `*testing.B`) through
the generated wrappers — always serially (it rejects `--parallel`, exit
2), because concurrent benchmarks time contention instead of code.
`-test.benchmem` is on by default.

Writing one: put the measured work inside `for b.Loop() { … }` —
`*gotest.B` has no `N`, timer or `Run` methods (`b.B()` reaches the
`*testing.B` for those; for `Loop`, `ReportAllocs`, `SetBytes`,
`ReportMetric`, `Name`, `Setenv` and `TempDir` call the `*gotest.B` itself
— `t-escape` flags the escape). Setup above the loop is not timed, nor are the suite's
`BeforeEach`/`AfterEach` around each benchmark. Assertions take `b`.
Lint: `bench-loop` (integrity) — the method never calls `b.Loop()`, so
nothing is measured; `bench-wait` — `time.Sleep`/`Eventually`/
`Consistently` inside the loop times the wait; `bench-fixture-io` — a
fixture field read inside the loop times whatever backs the fixture, so
hoist it above. A suite with benchmarks refuses a returning
`BeforeEach`, hooks taking `*testing.T`, and a hook calling `t.T()`
(nil in every hook under a benchmark) without checking it for nil. Flags:

- `--spec` — render the spec view; under GitHub Actions the markdown
  (with delta table) lands in the step summary automatically.
- `--save=<path>` — write this run as a JSON baseline. Bare `--save=`
  (empty value) falls back to `bench.baseline` from `.gotest.yml` and
  errors when neither names a path.
- `--against=<path>` — compare against a saved baseline and render the
  delta table (significant rows only unless `-v`; defaults to
  `bench.baseline`). Significance is Welch-tested, so run with `-count`
  high enough to give it samples. A baseline whose `goos`, `goarch`,
  `goVersion`, `cpu` or `gomaxprocs` differs from the run warns on stderr
  (`WARN: baseline was recorded in a different environment`) and reports the same under
  `envMismatch` in `--json`; the comparison still runs and the exit code
  is unchanged.
- `--gate=<pct>` — exit 1 when the worst significant regression exceeds
  the threshold (needs `--against` or `bench.baseline`, and `-count=4` or
  more: below that the gate exits 2 instead of judging on a rule of
  thumb). Without a gate, a comparison with fewer than 4 samples a side
  still prints; each such row ends in `(n<4, ±20% heuristic)`.
- `--json` — emit ONE versioned report document to stdout instead of
  human output: `schemaVersion` 1, the run's results in baseline shape,
  `deltas` when a comparison ran, `gate` with `breachedKeys` (every
  significant delta above the threshold) when a gate was active, and
  `envMismatch` when the baseline came from elsewhere. Consume this,
  never scrape text.
- Scoping: `-bench` matches the generated `Benchmark<Suite>` wrapper by
  its first slash segment; later segments select methods —
  `-bench='^BenchmarkFooTestSuite$/^BenchmarkParse$'` runs one method.
  `-benchtime=100x|2s` and `-count=<n>` pass through.

Baseline workflow: `bench --save= -count=4` on the trunk build; `bench
--against= --gate=10 -count=4` (or `bench.gate` in `.gotest.yml`) on branches;
promote a new baseline by re-running `--save=` after accepting a change.
In CI, prefer the action's `bench`/`bench-baseline`/`bench-gate`/
`bench-save` inputs (see `ci.md`).

## Fuzzing — `gotest fuzz` (v1.29+)

Suite methods named `Fuzz*` (`func (s *X) FuzzParse(f *gotest.F)`) declare
fuzz targets; inside, `f.Fuzz(func(t *gotest.T, in …) { … })` binds the
callback — any number of arguments after the `*gotest.T`, checked when
gotest generates. Plain `gotest ./...` already replays every
`f.Add` seed and committed corpus entry as regular subtests — reach for
the subcommand only to SPEND TIME SEARCHING for new inputs.
`go test -fuzz` cannot see suite targets (generated wrappers need the
overlay), so the orchestrator runs each `Fuzz<Suite>_<Method>` wrapper as
its own `go test -fuzz` process:

- `go tool gotest fuzz ./...` — fuzz every discovered target. Flags:
  `--for=<dur>` splits an approximate wall-clock budget jobs-aware across
  targets (default 1m; `--for=0` fuzzes
  until interrupted; per-target share floors at 10s; the schedule prints
  up front);
  `--jobs=<n>` (default max(1, GOMAXPROCS/2)) bounds concurrent targets;
  `--target=<FuzzSuite_Method>` fuzzes exactly one wrapper (unmatched
  names error with the available list); `--no-harvest` disables
  table-test seed harvesting for the run. `--for` is the session's only
  clock — the deadline follows it — and `gotest fuzz` refuses `--timeout`.
- Every session ends with one closing line (targets, execs, new
  interesting inputs, crashers); under GitHub Actions it also lands in the
  step summary as a per-target table.
- `go tool gotest fuzz triage ./...` — re-run each on-disk crasher under
  `testdata/fuzz/<Func>/`, print decoded input and cause; exit 1 while
  any still fails.
- `go tool gotest fuzz promote ./...` — splice each crasher into its
  method as a permanent typed `f.Add(...)` seed and delete the file —
  the durable form, especially for struct-typed targets whose corpus
  files are bound to the type's field order (the `fuzz-struct-corpus`
  lint rule flags them). v1.36.1+: only the crashers the latest
  `gotest fuzz` session wrote; an older corpus file is deleted or written
  as a typed `f.Add` seed by hand.

Any non-native argument, in any position, fuzzes through a generated fan
— one engine argument per leaf field. Write typed `f.Add` literals, never
raw `[]byte` seeds (`fuzz-raw-seed`; a wrong-typed seed is rejected at
`f.Fuzz`), keep
targets deterministic (`fuzz-determinism`), give the callback a property
to assert (`fuzz-no-oracle`), and keep per-execution hooks IO-free
(`fuzz-hook-io`).

Machine-readable capture, verified end-to-end:

```sh
go tool gotest -json ./... > events.json
grep -o '"Package":"[^"]*","Test":"[^"]*"' events.json | sort -u > cases.txt
grep -q 'TestProbeTargetTestSuite/TestAddsNumbers' cases.txt
grep -q 'two_plus_two' cases.txt
go tool gotest spec --input events.json > /dev/null
```

**v1.29.1+ census:** after a green `spec`, `summary` or `-json` run, every
suite method the source declares must have a verdict in the event stream.
If one is missing the run exits 2 with `FAIL: census: N declared test(s)
never ran` and the list: a harness that silently dropped a test is not a
passed test but an untrustworthy run. `bench --spec`, `--json`, `--save`
and `--against` runs apply the same rule to declared benchmark methods
(`... declared benchmark(s) never ran`). The census only judges green,
unfiltered runs; `-run`, `-skip`, `-list` and, for bench, `-bench` print
`note: census skipped` instead. The plain text run is never censused, so
gate CI on `summary`, `spec`, `-json` or a capturing `bench` run.

**v1.30+ census bookings:** a unit the census finds missing is also booked
into the event stream as a test that ran and failed, with
`census: declared test never ran` as its output, and its suite and package
fail with it. Every run that writes the event stream carries the booking
(`spec`, `summary`, `-json`, `watch --spec`, `watch -- -json`, a capturing
`bench`), so a stream consumer (the editor's Spec View included) can name the
method that vanished instead of seeing "all passed" beside exit 2. A run cut
short is not censused: an interrupt exits 130, and a `--timeout` that expires
before the last verdict exits 1 on every command. It prints `FAIL: global
--timeout exceeded after <d> while running: <pkg> <test>` (up to five, then
`… N more`), naming the hung method, or only its suite in a plain text test
run. A suite the deadline stopped fails its package even when every method
it ran passed, and the FAIL line names that suite when no method was still
running (v1.38+); `gotest bench` names the hung benchmark in text and `--json` alike. A
suite that hangs in its `BeforeAll`, or in a package fixture's `BeforeAll`, is
named as the suite. A test whose suite process ended on its own, or that was
still waiting for a `-parallel` slot, is never named. Each one is booked into
the stream as failed with `gotest: --timeout expired while this test was
running` as its output, and its suite and package fail with it. A deadline
that hits during compilation or shared fixture setup, before any suite
process starts, is booked as a failed `global --timeout` package.
