package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/about"
)

func containsHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--help" || a == "-h" || a == "-help" {
			return true
		}
	}
	return false
}

func showHelp(topic string) {
	switch topic {
	case "", "help":
		printUsage()
	case "test":
		printTestHelp()
	case "spec":
		printSpecHelp()
	case "summary":
		printSummaryHelp()
	case "watch":
		printWatchHelp()
	case "bench":
		printBenchHelp()
	case "fuzz":
		printFuzzHelp()
	case "discover":
		printDiscoverHelp()
	case "scaffold":
		printScaffoldHelp()
	case "lint":
		printLintHelp()
	case "migrate":
		printMigrateHelp()
	case "refactor":
		printRefactorHelp()
	case "skill":
		printSkillHelp()
	case "generate":
		printGenerateHelp()
	case "prepare":
		printPrepareHelp()
	case "clean":
		printCleanHelp()
	case "config":
		printConfigHelp()
	case "version":
		fmt.Println(about.LongInfo())
	default:
		fmt.Fprintf(os.Stderr, "unknown help topic: %s\n\n", topic)
		printUsage()
	}
}

func printUsage() {
	fmt.Printf(`%s — test suite runner for Go

Usage:
  gotest [flags] [--] [go-test-flags] [packages...]
  gotest <subcommand> [flags] [packages...]

Subcommands:
  bench       Run BenchmarkX suite methods serially
  fuzz        Run FuzzX suite methods with go test -fuzz
  spec        Render behavioral specification from test output
  summary     Show failure-focused test summary for CI
  watch       Watch for file changes and re-run tests
  discover    Discover test suites and output JSON
  scaffold    Generate test suite skeleton
  lint        Run gotest-specific linter checks
  migrate     Convert testify/suite tests to go-test format
  refactor    Source code refactoring tools
  skill       Install the agent skill matching this version
  generate    Run code generation only (no test execution)
  prepare     Start shared fixtures for debug (blocks until SIGTERM)
  clean       Remove orphaned generated files
  version     Print version information

Run "gotest help <subcommand>" for subcommand-specific help.

Flags (gotest — use --double-dash):
  --ci                    CI mode: fail on F_ prefixes, snapshot read-only
  --debug                 Keep the overlay and work dir for inspection;
                          a later run removes them after 7 days
  --spec                  Render spec view instead of default output
  --update-snapshots      Regenerate snapshot files
  --no-cache              Disable the test binary cache
  --min=<pct>             Fail if coverage < pct%% (0-100)
  --events-file=<file>    With -json, also write the go test -json event stream
                          to file as it happens
  --setup-timeout=<dur>   Total budget for shared fixture setup (default: the fixtures' declared
                          budgets, 30s until the first reports; 0 to disable)
  --timeout=<dur>         Global pipeline deadline (default: 15m, 0 to disable)
  --parallel=<n>          Total concurrent test method budget (default: 2×GOMAXPROCS,
                          halved under -race/-msan/-asan)
  --compile-parallel=<n>  Concurrent compilations (default: NumCPU, halved under
                          -race/-msan/-asan)
  --group=<name>          Run only the suites of this ExclusiveGroup (repeatable)
  --skip-group=<name>     Leave the suites of this ExclusiveGroup out (repeatable)
  --no-harvest            Disable fuzz seed harvesting for this run

Flags (go test — use -single-dash, forwarded automatically):
  -v                      Verbose output
  -run=<regexp>           Run only matching tests
  -skip=<regexp>          Skip matching tests (e.g. -skip='TestFooTestSuite/TestSlow')
  -count=<n>              Run each test n times (default: 1; no result is cached)
  -race                   Enable data race detector
  -timeout=<dur>          Deadline of each suite process (default: 10m); a
                          test's own deadline is SuiteConfig.Timeout
  -coverprofile=<file>    Write coverage profile
  -tags=<tags>            Build tags (comma-separated); --tags=<tags> too
  -json                   go test -json event stream; every matched package
                          gets a verdict, including those with no suites

Use a bare "--" to separate gotest flags from go test flags,
or to pass unrecognized flags to go test without validation.

Environment:
  CI=true                 Auto-enables --ci when GOTEST_CI is unset
  GOTEST_CI=0             Opt out of CI auto-detection
  GOTEST_CACHE_DIR=<dir>  Override the cache location (default: ~/.cache/gotest)
  GOTEST_SLOW_BUILD_THRESHOLD=<duration>
                          Report a build still running after this long
                          (default: 15s)

Configuration:
  Place a .gotest.yml in your project root. Run "gotest help config" for details.

Examples:
  gotest ./...                              Run all test suites
  gotest --ci --min=80 ./... -race          CI with coverage gate and race detection
  gotest spec --no-color ./...              Spec report for CI artifacts
  gotest watch ./pkg/auth/...               Re-run on file changes
  gotest scaffold ./pkg/auth.ServiceImpl    Generate test suite skeleton
  gotest lint ./...                         Check for common suite mistakes
`, about.ShortInfo())
}

func printTestHelp() {
	fmt.Print(`gotest test — run test suites (default subcommand)

Usage:
  gotest [flags] [--] [go-test-flags] [packages...]

When no subcommand is given, gotest discovers test suites in the target
packages, generates an overlay with lifecycle wiring, and runs each suite
in a process of its own. Test binaries are kept between runs, so a package
that did not change is not linked again.

Flags:
  --ci                    CI mode: fail on F_ prefixes, snapshot read-only
  --debug                 Keep the overlay and work dir for inspection;
                          a later run removes them after 7 days
  --spec                  Render spec view instead of default output
  --update-snapshots      Regenerate snapshot files
  --no-cache              Disable the test binary cache
  --no-harvest            Disable fuzz seed harvesting for this run (see below)
  --min=<pct>             Fail if coverage < pct% (0-100, enables -coverprofile)
  --events-file=<file>    With -json, also write the go test -json event stream
                          to file as it happens
  --setup-timeout=<dur>   Total budget for shared fixture setup (default: the fixtures' declared
                          budgets, 30s until the first reports; 0 to disable)
  --timeout=<dur>         Global pipeline deadline (default: 15m, 0 to disable)
  --parallel=<n>          Total concurrent test method budget (default: 2×GOMAXPROCS,
                          halved under -race/-msan/-asan)
  --compile-parallel=<n>  Concurrent compilations (default: NumCPU, halved under
                          -race/-msan/-asan)
  --group=<name>          Run only the suites of this ExclusiveGroup (repeatable)
  --skip-group=<name>     Leave the suites of this ExclusiveGroup out (repeatable)

All standard go test flags (-single-dash) are forwarded automatically; -run also
takes a Fuzz<Suite>_<Method> wrapper name, -p caps the suite processes, -exec wraps
every suite binary, and -o is refused (gotest builds the binaries itself).
Use a bare "--" to pass unrecognized flags without validation.

CI auto-detection: when the standard CI env var is set and GOTEST_CI is
unset, --ci is enabled automatically. Set GOTEST_CI=0 to opt out.

Every package a pattern matches ends in a verdict. A run whose patterns
match no package at all verified nothing: in every mode (spec, summary,
bench and fuzz too) it exits 2 and prints on stderr
  FAIL: "<pattern>" matched no packages
A pattern that matches nothing while others do only warns on stderr, as
go test does.
A test that panics or calls os.Exit fails its suite, os.Exit(0) included,
as under go test: the run exits 1, in every mode.
An empty pattern ("") is the current directory, as for go test. gotest
takes packages, not files: a .go file argument exits 2 and names it.

Fuzz seed harvesting (on by default): if any target package has FuzzX
suite methods, gotest mines your test files' table-test literals and
call-site arguments into f.Add(...) seeds when it generates their wrapper
— this happens as part of ordinary overlay generation, whether or not you
ever run "gotest fuzz". Pass --no-harvest to disable it for this run, or
set "fuzz: harvest: false" in .gotest.yml to disable it persistently (see
"gotest help config" and "gotest help fuzz").

Examples:
  gotest ./...                              Run all suites
  gotest ./pkg/auth/... -v                  Verbose output for one package
  gotest --ci --min=80 ./... -race          CI pipeline with coverage gate
  gotest --debug ./pkg/auth/...             Keep overlay for inspection
  gotest -- -customflag=value ./...         Flags only the test binary knows
`)
}

func printSpecHelp() {
	fmt.Print(`gotest spec — render behavioral specification from test suites

Usage:
  gotest spec [flags] [--] [go-test-flags] [packages...]
  gotest spec --input=<file> [--format=<fmt>] [--output=<file>]

Runs test suites and renders a BDD-style specification tree showing
pass/fail/skip status for each suite, method, and subtest.

A green run is believed only when every declared test method and fuzz
target produced a verdict. If one never ran, the run exits 2 with
"FAIL: census: N declared test(s) never ran" (or "fuzz target(s)") and the
missing names: a dropped test is not a failed test but a run that cannot be
trusted. Each missing unit is also booked into the event stream as a failed
test whose output is the census, and its suite and package fail with it, so
the rendered spec and -json consumers name it too. The census applies to
green runs only and stands down under -run, -skip and -list with a note on
stderr. Declared benchmarks a test run left unexecuted are counted with
"note: N benchmark(s) not run"; -bench runs them here instead. Patterns
that match no package exit 2 as well: a run of nothing is not green.

Flags:
  --format=<fmt>          Output format: terminal (default), md, json
  --output=<file>         Write to file instead of stdout
  --input=<file>          Render from saved JSON ("-" for stdin); exits 2 if
                          it holds no test events, a package or failed build
                          never reported its verdict (a stream cut short), or
                          a package failed to build (as the live run does)
  --render-only           With --input, exit 0 on a failing or incomplete
                          stream: report whether rendering succeeded, not the
                          test verdict
  --static                Read the specification from source without running
                          the suites; nodes carry no verdict
  --no-color              Disable ANSI color codes
  --ci                    CI mode: fail on F_ prefixes, snapshot read-only
  --debug                 Keep the overlay and work dir; a later run
                          removes them after 7 days
  --update-snapshots      Regenerate snapshot files
  --no-cache              Disable the test binary cache
  --min=<pct>             Fail if coverage < pct% (0-100)
  --setup-timeout=<dur>   Total budget for shared fixture setup (default: the fixtures' declared
                          budgets, 30s until the first reports; 0 to disable)
  --timeout=<dur>         Global pipeline deadline (default: 15m, 0 to disable)
  --parallel=<n>          Total concurrent test method budget (default: 2×GOMAXPROCS,
                          halved under -race/-msan/-asan)
  --compile-parallel=<n>  Concurrent compilations (default: NumCPU, halved under
                          -race/-msan/-asan)
  --group=<name>          Run only the suites of this ExclusiveGroup (repeatable)
  --skip-group=<name>     Leave the suites of this ExclusiveGroup out (repeatable)

Examples:
  gotest spec ./...                                Run and render spec
  gotest spec --format=md --output=spec.md ./...   Markdown report to file
  gotest spec --no-color --output=spec.txt ./...   Plain text for CI artifacts
  gotest spec --input=- < events.json              Render from piped JSON input
  gotest spec --format=json ./...                  JSON tree
`)
}

func printSummaryHelp() {
	fmt.Print(`gotest summary — failure-focused test summary for CI

Usage:
  gotest summary [flags] [--] [go-test-flags] [packages...]
  gotest summary --input=<file> [--format=<fmt>] [--output=<file>]

Runs test suites and shows only failing tests with their error output.
Designed for CI pipelines where the full spec tree is too verbose.

When all tests pass, prints a single success line. When tests fail,
prints each failure with its package, test path, and assertion output.

A green run is believed only when every declared test method and fuzz
target produced a verdict. If one never ran, the run exits 2 with
"FAIL: census: N declared test(s) never ran" (or "fuzz target(s)") and the
missing names: a dropped test is not a failed test but a run that cannot be
trusted. Each missing unit is also booked into the event stream as a failed
test whose output is the census, and its suite and package fail with it, so
the rendered spec and -json consumers name it too. The census applies to
green runs only and stands down under -run, -skip and -list with a note on
stderr. Declared benchmarks a test run left unexecuted are counted with
"note: N benchmark(s) not run"; -bench runs them here instead. Patterns
that match no package exit 2 as well: a run of nothing is not green.

Flags:
  --format=<fmt>          Output format: terminal (default), md, json
  --output=<file>         Write to file instead of stdout
  --events-file=<file>    Also write the run's go test -json event stream to
                          file as it happens; summary --input reads it back
  --input=<file>          Read from saved JSON ("-" for stdin); exits 2 if
                          it holds no test events, a package or failed build
                          never reported its verdict (a stream cut short), or
                          a package failed to build (as the live run does)
  --render-only           With --input, exit 0 on a failing or incomplete
                          stream: report whether rendering succeeded, not the
                          test verdict
  --no-color              Disable ANSI color codes
  --github                GitHub CI mode: emit annotations and step summary
                          (auto-detected via $GITHUB_ACTIONS)
  --coverage=<file>       Include coverage from profile in summary output
  --badge=<file>          Write a coverage badge SVG (enables -coverprofile)
  --ci                    CI mode: fail on F_ prefixes, snapshot read-only
  --debug                 Keep the overlay and work dir; a later run
                          removes them after 7 days
  --update-snapshots      Regenerate snapshot files
  --no-cache              Disable the test binary cache
  --min=<pct>             Fail if coverage < pct% (0-100)
  --setup-timeout=<dur>   Total budget for shared fixture setup (default: the fixtures' declared
                          budgets, 30s until the first reports; 0 to disable)
  --timeout=<dur>         Global pipeline deadline (default: 15m, 0 to disable)
  --parallel=<n>          Total concurrent test method budget (default: 2×GOMAXPROCS,
                          halved under -race/-msan/-asan)
  --compile-parallel=<n>  Concurrent compilations (default: NumCPU, halved under
                          -race/-msan/-asan)
  --group=<name>          Run only the suites of this ExclusiveGroup (repeatable)
  --skip-group=<name>     Leave the suites of this ExclusiveGroup out (repeatable)

GitHub CI mode (--github):
  Emits ::error annotations for each failure, showing inline on PR diffs.
  Writes a markdown summary to $GITHUB_STEP_SUMMARY.

Examples:
  gotest summary ./...                                Run and show failures only
  gotest summary --github ./... -race                 CI pipeline with annotations
  gotest summary --format=md --output=summary.md      Markdown failure report
  gotest summary --input=- < events.json              Summarize from piped JSON
  go test -json ./... | gotest summary --input=-      Filter plain go test output
`)
}

func printWatchHelp() {
	fmt.Print(`gotest watch — watch for file changes and re-run tests

Usage:
  gotest watch [flags] [--] [go-test-flags] [packages...]

Watches .go files under the target packages and re-runs affected tests
on every change. The initial run covers all target packages; subsequent
runs are scoped to the packages containing changed files. A run whose
patterns match no package yet is reported like a failed load, and watch
keeps watching: the first package written there runs.

Flags:
  --debounce=<dur>        Re-run delay after change (default: 200ms)
  --ci                    CI mode: fail on F_ prefixes, snapshot read-only
  --debug                 Keep the overlay and work dir; a later run
                          removes them after 7 days
  --spec                  Render spec view after each run
  --bench                 Benchmark mode: re-run benchmarks on change, with the
                          ns/op delta against the previous run
  --update-snapshots      Regenerate snapshot files
  --no-cache              Disable the test binary cache
  --setup-timeout=<dur>   Total budget for shared fixture setup (default: the fixtures' declared
                          budgets, 30s until the first reports; 0 to disable)
  --timeout=<dur>         Global pipeline deadline (default: 15m, 0 to disable)
  --parallel=<n>          Total concurrent test method budget (default: 2×GOMAXPROCS,
                          halved under -race/-msan/-asan)
  --compile-parallel=<n>  Concurrent compilations (default: NumCPU, halved under
                          -race/-msan/-asan)

Examples:
  gotest watch ./...                         Watch all packages
  gotest watch ./pkg/auth/... -v             Watch with verbose output
  gotest watch ./... --spec                  Spec view on every change
  gotest watch --debounce=500ms ./...        Slower debounce for large projects
`)
}

func printBenchHelp() {
	fmt.Print(`gotest bench — run BenchmarkX suite methods

Usage:
  gotest bench [flags] [--] [go-test-flags] [packages...]

Discovers suites containing BenchmarkX methods and runs each suite's
generated Benchmark<SuiteName> wrapper via "go test -bench". Benchmark
suites always dispatch serially, one at a time, regardless of --parallel
or -test.parallel — running benchmarks concurrently would make their
timing results meaningless. --min is not available in bench mode;
-coverprofile is still supported and profiles are merged as usual.

The standard go test flag -test.benchmem is enabled by default, so every
benchmark line reports allocations (B/op, allocs/op) alongside ns/op.

Flags:
  --spec                  Render spec view instead of raw benchmark output
  --no-color              Disable ANSI color codes (with --spec)
  --debug                 Keep the overlay and work dir for inspection;
                          a later run removes them after 7 days
  --no-cache              Disable the test binary cache
  --setup-timeout=<dur>   Total budget for shared fixture setup (default: the fixtures' declared
                          budgets, 30s until the first reports; 0 to disable)
  --timeout=<dur>         Global pipeline deadline (default: 15m, 0 to disable)
  --save=<path>           Save this run's results as a JSON baseline (forces
                           capture mode; the spec view is still rendered so
                           results remain visible). A bare --save= saves to
                           bench.baseline from .gotest.yml; saving to the
                           configured baseline promotes the run and compares
                           it against nothing, while an explicit --against
                           naming the --save file is refused (exit 2)
  --against=<path>        Compare this run against a saved baseline and print
                           a delta table (defaults to bench.baseline in
                           .gotest.yml when omitted)
  --gate=<pct>            Fail (exit 1) if the worst significant regression
                           against --against exceeds pct percent (e.g. 10 for
                           10%); requires --against or bench.baseline in
                           .gotest.yml and -count=4 or more (exit 2 below
                           that; defaults to bench.gate in .gotest.yml,
                           0 disables)
  --json                  Emit one versioned JSON document to stdout instead
                           of human output: the run's results (baseline
                           shape), every delta when a comparison ran
                           (significant or not), and the gate verdict when a
                           gate is active. Suppresses --spec rendering;
                           intended for tooling (the VS Code extension).

All standard go test flags (-single-dash) are forwarded automatically.
Use a bare "--" to pass unrecognized flags without validation.

Filtering:
  -bench=<regexp>         Select which benchmark suites run, matched against
                           each suite's Benchmark<SuiteName> wrapper name.
                           Sub-benchmark segments scope to single methods:
                           -bench='^BenchmarkFooTestSuite$/^BenchmarkParse$'
                           runs only that method, since the generated wrapper
                           runs each method under b.Run with its method name
  -benchtime=<x>          Iterations or duration per benchmark (e.g. 100x, 2s)
  -count=<n>              Run each benchmark n times

Note: -run and -bench both filter which suites run in bench mode, matched
against each suite's Benchmark<SuiteName> wrapper name — -run the same
way it scopes suites for "gotest test", -bench by its pattern's first
slash segment (later segments select sub-benchmarks inside the wrapper).
When both are given, a suite must match both to run (e.g.
"gotest bench ./pkg/parser -run Parse" filters by suite).

If no matched package contains any BenchmarkX methods, prints "no
benchmarks found" and exits 0 without invoking go test. Otherwise every
matched package ends in a verdict: one that runs no benchmark passes as
under go test -bench ("ok  <pkg>  0.000s"). Patterns that match no
package at all exit 2.

A capturing run (--spec, --json, --save or --against) is believed only
when every declared BenchmarkX method reported a result. If one never ran,
the run exits 2 with "FAIL: census: N declared benchmark(s) never ran" and
books each missing benchmark into the captured stream as a failed one;
-run and -bench stand the census down with a note on stderr.

--against prints a delta table (BENCHMARK / OLD ns/op / NEW ns/op / Δ)
comparing each benchmark's new mean ns/op against the saved baseline's.
Only statistically significant deltas are shown by default; pass -v to
show every row. Significant regressions are marked with a trailing "⚠".
Significance is a Welch t-test over -count samples; with fewer than 4 a
side it is a ±20% rule of thumb and the row says "(n<4, ±20% heuristic)".
Deltas alone never change the exit code — only --gate does, and a gate
refuses to judge below -count=4.

A baseline records the goos, goarch and Go version that produced it. When
any of them differ from this run, --against prints "WARN: baseline was
recorded in a different environment" naming each field, and the --json
report carries the same under "envMismatch". The comparison still runs and
the exit code is unchanged: the deltas then measure the machines as much as
the code, and only you know whether the two are alike enough to mean
something.

Examples:
  gotest bench ./...                          Run all benchmark suites
  gotest bench ./pkg/auth/... -benchtime=2s   Longer per-benchmark budget
  gotest bench -bench=Cache ./...             Only suites matching "Cache"
  gotest bench --spec ./...                   Spec-style rendering
  gotest bench --save=bench.json ./...        Save results as a baseline
  gotest bench --against=bench.json ./...     Compare against a baseline
  gotest bench --against=bench.json --gate=10 ./...
                                               Fail if any benchmark regresses >10%
`)
}

func printFuzzHelp() {
	fmt.Print(`gotest fuzz — search for failing inputs with FuzzX suite methods

Usage:
  gotest fuzz [flags] [packages...]
  gotest fuzz triage [packages...]     Re-run each crasher, report if it still fails
  gotest fuzz promote [packages...]    Splice crashers into f.Add(...) seeds

Runs every Fuzz<SuiteName>_<MethodName> wrapper the target packages
generate as its own "go test -fuzz" process, several at a time (--jobs),
for a shared time budget (--for). Output streams live, one line at a
time, prefixed "[<Func>] ".

Seeds already replay without this subcommand: every f.Add seed and every
corpus entry under testdata/fuzz/ runs as an ordinary subtest of a plain
"gotest" run. Use "gotest fuzz" to spend time searching for new inputs.

Flags:
  --for=<dur>             Wall-clock budget for the whole session (default 1m).
                          Each target gets --for × min(--jobs, targets) / targets,
                          never less than 10s; the resolved schedule prints
                          before the search starts. --for=0 removes the budget:
                          targets fuzz until interrupted.
  --jobs=<n>              Max concurrent targets (default: max(1, GOMAXPROCS/2))
  --target=<Fuzz...>      Fuzz one target, named by its wrapper as printed in
                          session output; an unmatched name lists the targets
                          (-run, -skip and -bench are refused in its favour)
  --no-harvest            Do not mine table-test literals into seeds for this run
  --no-cache              Disable the test binary cache
  --debug                 Keep the overlay and work dir for inspection;
                          a later run removes them after 7 days

--for is the session's only clock: the deadline follows it, so --timeout
is refused here. With --for=0 only the first --jobs targets run, since
each holds its slot until interrupted.

Exit codes:
  0   The budget ran out or the session was interrupted with nothing found
  1   A finding: a failing target or a new crasher file
  2   The session could not run as requested

A new crasher is named as it is found:
  [FuzzParserTestSuite_FuzzParse] new crasher: <pkg>/testdata/fuzz/FuzzParserTestSuite_FuzzParse/1a2b3c
Inspect it with "gotest fuzz triage", keep it with "gotest fuzz promote".
Every session closes with one line (targets, executions, new interesting
inputs, crashers); under GitHub Actions the same session is appended to
$GITHUB_STEP_SUMMARY as a table. With no FuzzX methods in the matched
packages, the command prints "no fuzz targets found" and exits 0; patterns
that match no package at all exit 2.

Triage:
  "gotest fuzz triage" scans each target's testdata/fuzz/<Func>/ directory
  and re-runs every entry via "go test -run='^<Func>/<hash>$'", printing
  its decoded input and the failure cause, or "status: no longer failing":
    FuzzParserTestSuite_FuzzParse: 1 crasher
      file:  testdata/fuzz/FuzzParserTestSuite_FuzzParse/1a2b3c
      input: string("a@\x00")
      cause: panic: runtime error: index out of range [3] with length 3
  Exits 1 while any crasher still fails, 0 otherwise. Entries of a type it
  cannot decode are reported and skipped, one file at a time.

Promote:
  "gotest fuzz promote" splices each crasher the latest "gotest fuzz"
  session of its target wrote into its FuzzX method as a permanent
  f.Add(...) seed of the crasher's exact values, after the method's last
  f.Add, and deletes the file; the seed then replays on every ordinary run:
    promoted FuzzParserTestSuite_FuzzParse/1a2b3c -> f.Add("a@\x00") in parser_test.go:42
  Other corpus files are left in place. A session crasher that is not in
  Go's corpus format, or changed since the session, promotes nothing and
  exits 1. A struct target's seed is the value its codec decodes; a
  crasher whose method cannot be located with confidence is left in place
  with a warning; promote never partially edits source.

Seed harvesting is on by default: literal arguments from table tests and
call sites in _test.go files become extra f.Add seeds at generation time
(--no-harvest, or "fuzz: harvest: false" in .gotest.yml). "gotest lint"
checks fuzz targets with the fuzz-* rules (see "gotest help lint"). Why
each target needs its own "go test -fuzz" process, and how struct
arguments fan out, is in the README's Fuzzing section.

Examples:
  gotest fuzz ./pkg/parser/...                Fuzz for the default minute
  gotest fuzz --for=5m ./...                  About five minutes across all targets
  gotest fuzz --for=1m --jobs=2 ./...         Two targets at a time
  gotest fuzz --target=FuzzParserTestSuite_FuzzParse ./pkg/parser
`)
}

func printDiscoverHelp() {
	fmt.Printf(`gotest discover — discover test suites and output JSON metadata

Usage:
  gotest discover [-tags=<tags>] [packages...]

Loads test packages, discovers suite types and their methods, and outputs
structured JSON on one line. Nothing is run. Used by IDE extensions for
test explorer integration.

Output schema:
  {
    "version": %q,
    "packages": [{
      "importPath": "example.com/pkg",
      "dir":        "/absolute/path",
      "modulePath": "example.com",
      "testOnly":   false,
      "broken":     false,
      "suites": [{
        "name":     "UserTestSuite",
        "file":     "user_test.go",
        "line":     10,
        "col":      1,
        "parallel": false,
        "focused":  false,
        "excluded": false,
        "guarded":  false,
        "lifecycle": ["BeforeAll", "AfterAll"],
        "fixtures":  ["E2ESetupFixture"],
        "methods": [{
          "name": "TestCreate",
          "file": "user_test.go",
          "line": 15,
          "col":  1,
          "focused":  false,
          "excluded": false,
          "parallel": false,
          "behaviors": [{
            "name": "email_is_valid", "display": "when email is valid",
            "kind": "when", "line": 16,
            "children": [{"name": "creates_the_user", "display": "creates the user",
                          "kind": "it", "line": 17}]
          }],
          "behaviorsComplete": true
        }],
        "benchmarks": [{"name": "BenchmarkCreate", "file": "user_test.go", "line": 40, "col": 1}],
        "fuzzers":    [{"name": "FuzzParse", "file": "user_test.go", "line": 50, "col": 1}]
      }]
    }],
    "warnings": [{"importPath": "...", "file": "...", "line": 1, "col": 1, "message": "..."}]
  }

"version" is the CLI that wrote the document. A package that fails to load
is listed with "broken": true and no suites. "behaviors" is the When/It
tree a method declares, read from source; "behaviorsComplete": false means
the method declares behaviors that depend on runtime values, so the list is
a floor. "benchmarks" and "fuzzers" carry the same fields as "methods",
without behaviors.

"warnings" names, with its position, what a run would refuse (a package
that does not load, an unsupported hook signature, a fuzz argument that
cannot be fuzzed) and what it would pass over in silence: a lifecycle hook
that is misspelled or carries X_, and a Benchmark or Fuzz method on a type
that is no test suite.

Exit codes: 0 when the document was written, including for patterns that
match no package ("packages": [], a warning on stderr: an editor's
workspace may hold no package yet); 2 when the packages cannot be loaded
at all.

Examples:
  gotest discover ./...                      All packages
  gotest discover ./pkg/auth/...             Single package tree
  gotest discover -tags=integration ./...    With build tags
`, about.ResolvedVersion())
}

func printScaffoldHelp() {
	fmt.Print(`gotest scaffold — generate test suite skeleton

Usage:
  gotest scaffold <target>
  gotest scaffold --fuzz <./pkg/path.FuncName>

Target is one of:
  ./pkg/path.TypeName      Generate suite for a specific type
  ./pkg/path/file.go       Generate a suite over the file's exported functions
  ./pkg/path.FuncName      With --fuzz: generate a fuzz skeleton for a
                           single-parameter package-level function

Creates a new _test.go file next to the target: a suite struct holding
the subject, a BeforeEach that constructs it, and one Test method per
exported method with It stubs (a happy path and an error case for
methods that return an error). Existing files are never overwritten.
A package path may be relative or absolute.

Scaffold writes only code that compiles. It refuses a generic type or
function, and a method of a generic type, since a suite would need type
arguments it cannot choose; and it refuses a suite name the package
already declares, naming that declaration and its file.

Flags:
  --fuzz    Scaffold a Fuzz<Func> method for a package-level function
            instead of a type/file suite (see below)

--fuzz (round-trip and crash-safety skeletons):

Looks for an inverse of the target function by name — Marshal<->Unmarshal,
Encode<->Decode, Parse<->Format, Parse<->String, and same-suffix prefix
pairs like ParseJSON<->FormatJSON — then checks the two signatures are
actually compatible inverses: f: A -> (B[, error]) and g: B -> (A[, error]),
modulo the error result, compared with go/types' Identical (so two named
types with the same underlying type never match).

When a compatible inverse is found and A is one of Go's natively fuzzable
types (string, []byte, bool, and the int/uint/float variants), the
generated method seeds a corpus entry and asserts the round-trip property
— decoding what was just encoded returns the original input:

  func (s *EncodeTestSuite) FuzzEncode(f *gotest.F) {
      f.Add("")
      f.Fuzz(func(t *gotest.T, in string) {
          encoded, err := Encode(in)
          if err != nil {
              return
          }
          decoded, err := Decode(encoded)
          gotest.NoError(t, err)
          gotest.Equal(t, in, decoded) // round-trip property
      })
  }

When no compatible inverse is found (but A is still fuzzable — natively,
or through a generated codec for a struct or named type), it falls back to
a crash-safety skeleton that only calls the function — no assertions
beyond "doesn't panic" — and prints:
  no inverse pair found for Encode — generated crash-safety skeleton

When gotest cannot fuzz A at all (a map, an interface, a struct with
unexported fields — the same shapes "gotest generate" rejects), scaffold
emits a stub method carrying the generator's own rejection reason, and
prints:
  cannot fuzz map[string]string for Encode — generated TODO stub: <the generator's reason>

Exit codes:
  0   The skeleton was written
  1   The target could not be scaffolded: not found, a method, generic,
      its suite name already declared, or its file exists
  2   Usage error: no target, more than one, a target naming no type or
      function, or an unknown flag

Examples:
  gotest scaffold ./pkg/auth.UserService        Suite for UserService type
  gotest scaffold ./pkg/auth/service.go         Suite over the file's exported functions
`)
}

func printLintHelp() {
	fmt.Print(`gotest lint — run gotest-specific linter checks

Usage:
  gotest lint [flags] [packages...]

Checks for common mistakes in gotest test suites. Defaults to ./... if
no packages are specified.

Rules:
  focus                 Focused (F_) suites/methods should not be committed
  receiver              Suite methods should use pointer receivers
  lifecycle-typo        Methods similar to lifecycle hooks (likely typos)
  lifecycle-pair        BeforeAll without matching AfterAll (resource leak)
  generated-file        Generated overlay files should not be in version control
  stdlib-test           Stdlib test functions — consider using gotest suites
  testify               testify imports — consider migrating to gotest
  poll-scope            Outer t used inside Eventually/Consistently callbacks
  test-signature        Test methods not accepting *gotest.T or *testing.T
  x-lifecycle           X_ prefix on a lifecycle hook (a no-op)
  suite-lifecycle       Cleanup/Parallel/Run via t.T() — bypass the suite lifecycle
  shared-fixture-undeclared
                        Reads of a shared fixture the suite never declared as a
                        field (only declared fixtures are started)
  generation-rejects    Shapes the generator rejects, where and as it rejects them
  nolint-unknown        //nolint ids that name no gotest rule or golangci-lint linter
  suite-declaration     Suites/fixtures declared inside a type ( … ) block
  no-embedding          Embedded suites, fixtures or hook-carrying types, and
                        suite-typed fields of a suite or fixture
  fixture-hook-call     Direct calls of a lifecycle hook the harness calls
  assertion-simplify    Simplifiable assertions (True(t, a == b) → Equal, …)
  assertion-type-guard  Nil/Empty on types their runtime guards reject
  assertion-redundant   Assertions made redundant by the following assertion
  fail-guard            if cond { Fail/Fatal(...) } guards — use assertions directly
  t-escape              Unnecessary t.T() convenience escapes (incl. Helper/Fatal/Log)
  behavior-wording      When("when …") / It("it …") — the spec supplies those words
  config-no-deadline    A negative SuiteConfig/FixtureConfig timeout — every
                        negative disables the deadline; say so by name
  bench-loop            Benchmark methods that never call b.Loop()/b.N (nothing
                        iterates, so the numbers lie)
  bench-fixture-io      Fixture-backed reads inside the measured loop (times the
                        fixture, not the code)
  bench-wait            time.Sleep/Eventually/Consistently inside the measured
                        loop (times the wait, not the code)
  fuzz-determinism      Fuzz targets reading time.Now/math-rand/os.Getenv
  fuzz-no-oracle        Fuzz callbacks that assert nothing (panic-only)
  fuzz-seed             Fuzz targets with no f.Add seeds
  fuzz-struct-corpus    On-disk corpus entries for struct-typed targets
                        (field-order-bound — delete them or write typed seeds)
  fuzz-hook-io          IO in BeforeEach/AfterEach of fuzzing suites
                        (hooks replay around every execution)
  fuzz-raw-seed         Raw []byte seeds on positions not taking []byte

Integrity rules can only be suppressed per line with //nolint. All other
rules also accept a project-wide skip flag (mirrored by .gotest.yml lint.skip):

Flags:
  -skip-<rule>            Disable a non-integrity rule, e.g. -skip-fail-guard
                          (assertion-simplify, assertion-redundant, behavior-wording,
                          config-no-deadline, bench-fixture-io, bench-wait,
                          fail-guard, t-escape, stdlib-test, testify,
                          fuzz-no-oracle, fuzz-seed, fuzz-hook-io, fuzz-raw-seed)
  -disable-nolint         Ignore //nolint comments
  -tags list              Build tags, a comma-separated list as for go build
                          (GOFLAGS=-tags=… works too)
  -fix                    Apply suggested fixes, overlapping ones included,
                          then report the findings left as a plain run
                          would (exit 3 if any)
  -diff                   Print what -fix would write as a unified diff on
                          stdout and write nothing; report and exit as -fix
                          would (implies -fix). Paths are a/ and b/ plus
                          the path from the current directory, so git
                          apply takes the diff there; a file outside it
                          gets a ../ path, which needs git apply
                          --unsafe-paths. A symlink is named by the file
                          it leads to, which -fix writes
  -json                   Print the findings as JSON on stdout and exit 0,
                          as the go/analysis driver does (also with -fix;
                          not with -diff)
  --github                Print the findings as GitHub ::error annotations in
                          place of the plain-text report, and append a findings
                          table to $GITHUB_STEP_SUMMARY. Implied when
                          GITHUB_ACTIONS=true; ignored alongside -json, -diff
                          and flags only the go/analysis driver has.

Suppress individual diagnostics with //nolint comments:
  //nolint:stdlib-test             Same line, or a comment block of its own
                                   ending on the line directly above
  package foo //nolint:stdlib-test  Suppress for entire file
  //nolint // reason               Every rule (so does //nolint:all)

A pattern that matches no package while others do only warns on stderr,
  warning: "<pattern>" matched no packages
as go does.

Exit codes:
  0   No findings (with -fix or -diff: none left after fixing)
  1   Nothing analyzable: uncompilable target packages (nothing was proven
      about them), patterns that match no package or name a directory or
      package that does not exist (said as go says it); or fixed sources
      that no longer compile, or fixed files that could not be written
      (-fix writes them all or none; on a failure stderr names any that
      were)
  2   Usage or configuration error (e.g. an integrity rule in lint.skip)
  3   Findings reported (with -fix or -diff: the findings left after fixing)
  130 -fix or -diff stopped by a signal; nothing was written (once the
      fixed files are being written, they all are first)

Examples:
  gotest lint ./...                          Lint all packages
  gotest lint -skip-testify ./...            Skip testify rule
  gotest lint -fix ./...                     Auto-fix where possible
  gotest lint -diff ./... | git apply        Preview the fixes, then apply them
`)
}

func printMigrateHelp() {
	fmt.Print(`gotest migrate — convert testify/suite tests to go-test format

Usage:
  gotest migrate [--dry-run] [packages...]

Scans for testify/suite patterns and rewrites them into gotest suite
format. Packages are named as directories (./pkg, ./..., or .\pkg on
Windows) or import paths of the main module; the current package if none
are given. A directory counts when it holds Go files under any build tag.
A package's test files are migrated together, its external _test package
included.

The result is type-checked for real, from the migrated sources, in the
default build and in each tag set the test files' //go:build lines ask
for. Anything it cannot convert, and every line that does not compile,
is left with a TODO(gotest-migrate) marker. The builds are the host's
GOOS and GOARCH: a test file only another OS builds is refused, so run
migrate on each platform whose test files differ. Generated test files
(Code generated ... DO NOT EDIT.) are skipped and named on stderr.

Exit codes:
  0  every migrated package compiles, no marker is left, and every hook
     and test testify ran is one gotest calls
  1  a marker is left, including one an earlier run placed
  2  a pattern names nothing, a package does not compile before the
     migration (it is left alone), or the migrated files cannot be
     written (none is: they are written all or none)

The module must require github.com/mvrahden/go-test (go get -tool does).

Flags:
  --dry-run    Print the diff of every edit instead of writing it

Examples:
  gotest migrate ./...                       Migrate all packages
  gotest migrate --dry-run ./pkg/auth        Show what one package would become
  gotest migrate example.com/app/...         Migrate by import path
`)
}

func printRefactorHelp() {
	fmt.Print(`gotest refactor — source code refactoring tools

Usage:
  gotest refactor <command> [args]

Commands:
  toggle-focus <file> <identifier>

    Toggle the F_ prefix on a suite type or method, declared in <file>,
    at every reference to it in the package and its test packages. The
    identifier is either "SuiteName" (toggles the suite) or
    "SuiteName.MethodName" (toggles a single method).

Examples:
  gotest refactor toggle-focus user_test.go UserTestSuite
  gotest refactor toggle-focus user_test.go UserTestSuite.TestCreate
`)
}

func printSkillHelp() {
	fmt.Print(`gotest skill — the agent skill for coding agents

Usage:
  gotest skill install [--dir=<skills-dir>]

Writes the writing-gotest-tests skill, as shipped with this gotest version,
into <skills-dir>/writing-gotest-tests, replacing an older copy. Rerun it
after upgrading gotest. A directory holding a different skill is refused,
and a failed install leaves the installed copy as it was (exit 2).

<skills-dir> is relative to the current directory. Agents read it from the
repository root, so where the Go module sits in a subdirectory, point --dir
at the root's skills directory (e.g. --dir=../.agents/skills).

Flags:
  --dir=<skills-dir>   Skills directory to install into (default: .agents/skills,
                       read by Copilot, Codex, Cursor, Gemini CLI,
                       Antigravity and others;
                       Claude Code reads .claude/skills)

Examples:
  gotest skill install
  gotest skill install --dir=.claude/skills
`)
}

func printGenerateHelp() {
	fmt.Print(`gotest generate — run code generation only

Usage:
  gotest generate [-tags=<tags>] [packages...]

Generates the overlay files that gotest normally creates transparently
during test runs. Useful for inspecting generated code or integrating
with custom build pipelines.

Files are written into main-module packages only (go.work modules count);
a matched dependency with suites is skipped with a note on stderr. A run
still runs its suites, from the overlay.

Exit codes: 0 when every matched package was generated for, including for
patterns that match no package (nothing to write, a warning on stderr, as
go generate does); 2 when a package does not build or generation fails.

Examples:
  gotest generate ./...                      Generate for all packages
  gotest generate -tags=integration ./...    With build tags
`)
}

func printPrepareHelp() {
	fmt.Print(`gotest prepare — prepare debug session

Usage:
  gotest prepare [packages...]

Generates the test overlay and starts shared fixtures, then blocks until
SIGTERM. Used by IDE extensions to prepare Delve debug sessions.

Outputs JSON to stdout:
  {"overlayFile": "...", "dir": "...", "stateFile": "..."}

Examples:
  gotest prepare ./pkg/auth/...              Prepare auth package for debug
`)
}

func printCleanHelp() {
	fmt.Print(`gotest clean — remove orphaned generated files

Usage:
  gotest clean [packages...]

Removes generated overlay files (gotest_p(x)suite_test.go, and the
ƒƒ_p(x)suite_test.go older versions wrote) from the source tree. A run never writes them there; "gotest generate" does, for
inspection. Patterns expand as for generate, so "clean ./..." removes what
"generate ./..." wrote; files in packages outside the main module(s) are
left with a note on stderr. Without a pattern it cleans the current
package only, as generate does; before v1.33 it walked the directory
tree. Above several modules, or for a module pulled in by a replace
directive, use a go.work that lists them, or run clean inside each.

The test binary cache (used for faster repeated runs) is managed
separately: each run's start evicts the binaries of a checkout that no
longer exists, those unused for 7 days, then the least recently used
above 5 GiB. Set GOTEST_CACHE_DIR to
control the cache location; use --no-cache to bypass it.

Examples:
  gotest clean ./...                         Clean all packages
`)
}

func printConfigHelp() {
	fmt.Print(`gotest configuration — .gotest.yml project settings

gotest reads project settings from a .gotest.yml file. The file is found
by walking up from the working directory, stopping at the first match or
at a go.mod boundary. CLI flags override config values.

Fields:
  tags: <string>            Build tags, comma-separated (e.g., "integration,e2e")
  setup-timeout: <duration> Total budget for all shared fixture setup (default: the fixtures' declared budgets, 30s until the first reports; 0 to disable)
  timeout: <duration>       Global pipeline deadline (default: 15m, 0 to disable)
  min-coverage: <int>       Minimum coverage percentage, 0-100
  parallel: <int>           Total concurrent test method budget (default: 2×GOMAXPROCS, auto-halved for -race/-msan/-asan)
  compile-parallel: <int>   Concurrent compilation processes (default: NumCPU, auto-halved for -race/-msan/-asan)
  debounce: <duration>      Watch mode re-run delay (e.g., "500ms", default: 200ms)
  lint:
    skip: [<rule>, ...]     Lint rules to disable globally
  bench:
    baseline: <path>        Default --against baseline path for "gotest bench"
    gate: <float>           Default --gate regression percentage (0 disables)
  fuzz:
    harvest: <bool>         Seed harvesting for "gotest fuzz" (default: true;
                             the --no-harvest CLI flag overrides this per-run)

Skippable lint rules (non-integrity only): assertion-redundant,
assertion-simplify, behavior-wording, bench-fixture-io, bench-wait,
config-no-deadline, fail-guard, fuzz-hook-io, fuzz-no-oracle,
fuzz-raw-seed, fuzz-seed, stdlib-test, t-escape, testify

Example .gotest.yml:

  tags: integration
  setup-timeout: 2m
  min-coverage: 80
  lint:
    skip:
      - testify
  bench:
    baseline: bench-baseline.json
    gate: 10
  fuzz:
    harvest: false
`)
}
