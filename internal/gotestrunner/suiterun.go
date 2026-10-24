package gotestrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// SuiteSpec identifies a test suite by its package and name. It carries only
// the immutable identity fields — the "what to run" — without execution details.
type SuiteSpec struct {
	Package   string // import path (for reporting)
	Dir       string // package source directory (working dir for the binary)
	SuiteName string // test function name, e.g., "TestFooTestSuite"
	RunFilter string // raw -test.run value (overrides SuiteName if set)
}

// SuiteTarget identifies a single test suite (or group of standalone tests)
// to run in its own subprocess.
type SuiteTarget struct {
	SuiteSpec
	BinaryPath   string   // path to compiled test binary
	RunFlags     []string // test binary flags (with -test. prefix)
	CoverProfile string   // per-suite cover profile path (empty if no -coverprofile)
	BudgetFile   string   // sidecar path for teardown budget (empty = use default)
	StopFile     string   // created before the process is interrupted, so it tears its fixtures down (empty = none)
	Bench        bool     // when true, run the Benchmark<SuiteName> wrapper instead of the suite's tests
	BenchFilter  string   // raw -test.bench value carrying the user's sub-benchmark segments (empty = the exact Benchmark<SuiteName> wrapper)
	Exclusive    bool     // SuiteConfig{Exclusive: true}: dispatched strictly alone, after every non-exclusive suite
	FuzzFuncs    []string // generated Fuzz<Suite>_<Method> wrapper names whose seed corpus replays alongside SuiteName in a plain run (ignored when Bench is true or RunFilter is set)
	Exec         []string // go test's -exec: the command the binary runs under (empty = the binary itself)
}

// SuiteResult holds the output from running a single suite subprocess.
type SuiteResult struct {
	Target   SuiteTarget
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	// Started is when the process was started; Duration how long it ran.
	// Together they give a package the clock its suites occupied.
	Started  time.Time
	Duration time.Duration
	// CutShort reports that the run's context ended while the process ran.
	CutShort bool
}

// RunMode controls how a run executes its suites and collects their output.
type RunMode int

const (
	// RunBatchText runs suites directly (no test2json). Output is batched
	// per-package: when all suites for a package complete, their output is
	// written to stdout followed by a package summary line.
	RunBatchText RunMode = iota

	// RunStreamJSON wraps each suite with test2json and streams its JSON
	// events to stdout as soon as it completes.
	RunStreamJSON

	// RunCaptureJSON wraps each suite with test2json and captures all JSON
	// events into a buffer (nothing written to stdout).
	RunCaptureJSON
)

// SortTargetsSerial orders targets in place by (Package, SuiteName) — the
// deterministic dispatch order RunBenchSuites executes them in.
func SortTargetsSerial(targets []SuiteTarget) {
	sort.Slice(targets, func(a, b int) bool {
		if targets[a].Package != targets[b].Package {
			return targets[a].Package < targets[b].Package
		}
		return targets[a].SuiteName < targets[b].SuiteName
	}) //nolint:gocritic // mirror of sortTargetIndices over the targets themselves
}

// RunBenchSuites executes bench targets strictly one at a time, in slice
// order (see SortTargetsSerial): benchmarks own the machine, and their
// verdicts are wall-clock measurements no concurrent suite may corrupt.
// beforeSlot/afterSlot (nil-safe) bracket each slot with its index, so the
// caller can open and close per-slot fixture windows. envFor gives a slot
// its environment, or the reason it is given up: a target whose fixture
// never came up is booked as failed with that reason and never runs.
func RunBenchSuites(ctx context.Context, targets []SuiteTarget, collector *OutputCollector, beforeSlot, afterSlot func(i int), envFor func(i int) (env []string, giveUp string)) {
	pkgCount := map[string]int{}
	var pkgOrder []string
	localIdx := make([]int, len(targets))
	for i := range targets {
		if _, seen := pkgCount[targets[i].Package]; !seen {
			pkgOrder = append(pkgOrder, targets[i].Package)
		}
		localIdx[i] = pkgCount[targets[i].Package]
		pkgCount[targets[i].Package]++
	}
	for _, pkg := range pkgOrder {
		collector.Register(pkg, pkgCount[pkg])
	}

	useTest2JSON := collector.UsesTest2JSON()

	for i := range targets {
		if ctx.Err() != nil {
			return
		}
		if beforeSlot != nil {
			beforeSlot(i)
		}
		if env, giveUp := envFor(i); giveUp != "" {
			collector.RecordGivenUp(targets[i].Package, localIdx[i], protocol.PrefixBenchmark+targets[i].SuiteName, giveUp)
		} else {
			r := RunSingleSuite(ctx, targets[i], env, useTest2JSON)
			collector.RecordResult(targets[i].Package, localIdx[i], r)
		}
		if afterSlot != nil {
			afterSlot(i)
		}
	}
}

func buildSuiteCmd(ctx context.Context, target SuiteTarget, env []string, test2json bool) *exec.Cmd { //nolint:gocritic // hugeParam: stable API
	// As go test does: os.Exit(0) during a test panics instead of passing.
	testArgs := []string{"-test.paniconexit0"}
	switch {
	case target.Bench:
		// BenchFilter carries a user -bench pattern with sub-benchmark
		// segments ("^BenchmarkSuite$/^BenchmarkMethod$"): the generated
		// wrapper runs each method under b.Run with its method name, so go
		// test's own slash matching scopes the run to single methods. The
		// suite was already selected by the pattern's first segment in
		// BuildBenchTargets; without segments the exact wrapper name runs
		// every method, as before.
		benchArg := fmt.Sprintf("-test.bench=^Benchmark%s$", regexp.QuoteMeta(target.SuiteName))
		if target.BenchFilter != "" {
			benchArg = "-test.bench=" + target.BenchFilter
		}
		testArgs = append(testArgs, "-test.run=^$", benchArg)
		if !slices.Contains(target.RunFlags, "-test.benchmem") {
			testArgs = append(testArgs, "-test.benchmem")
		}
	case target.RunFilter != "":
		// A user-supplied -run filter already becomes the entire -test.run
		// value, so it is left untouched: fuzz seeds replay only when a
		// generated Fuzz<Suite>_<Method> name happens to match the user's
		// filter on its own merit. We do not widen an explicit user filter
		// to also include fuzz funcs — user filter wins (controller ruling).
		testArgs = append(testArgs, "-test.run="+target.RunFilter)
	case len(target.FuzzFuncs) > 0:
		// No user filter: widen the run to an alternation of the suite's
		// Test function and every generated Fuzz<Suite>_<Method> wrapper, so
		// each fuzz func's seed corpus replays as ordinary subtests under a
		// plain run (zero-cost regression testing). target.SuiteName already
		// carries the "Test" prefix (see SuiteSpec.SuiteName), so it is used
		// as-is here rather than re-prefixed.
		alternates := append([]string{regexp.QuoteMeta(target.SuiteName)}, quoteAll(target.FuzzFuncs)...)
		testArgs = append(testArgs, fmt.Sprintf("-test.run=^(?:%s)$", strings.Join(alternates, "|")))
	default:
		testArgs = append(testArgs, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}

	if test2json {
		testArgs = append(testArgs, "-test.v=test2json")
		for _, f := range target.RunFlags {
			if f == "-test.v" || strings.HasPrefix(f, "-test.v=") {
				continue
			}
			testArgs = append(testArgs, f)
		}
	} else {
		testArgs = append(testArgs, target.RunFlags...)
	}

	if target.CoverProfile != "" {
		testArgs = insertBeforeWords(testArgs, "-test.coverprofile="+target.CoverProfile)
	}

	if test2json {
		argv := test2jsonArgv(test2jsonPath(), target, testArgs)
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: the Go toolchain's own converter
		cmd.Env = slices.Concat(env, sidecarEnv(target))
		cmd.Dir = target.Dir
		return cmd
	}

	argv := append(slices.Clone(target.Exec), target.BinaryPath)
	argv = append(argv, testArgs...)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: binary built by this tool, under the -exec command the user named
	cmd.Env = slices.Concat(env, sidecarEnv(target))
	cmd.Dir = target.Dir
	return cmd
}

// sidecarEnv names the files a suite process shares with the runner.
func sidecarEnv(target SuiteTarget) []string { //nolint:gocritic // hugeParam: stable API
	var env []string
	if target.BudgetFile != "" {
		env = append(env, protocol.EnvTeardownBudgetFile+"="+target.BudgetFile)
	}
	if target.StopFile != "" {
		env = append(env, protocol.EnvStopFile+"="+target.StopFile)
	}
	return env
}

// startRetryFirst and startRetryLimit bound the retries of a suite process
// whose executable is busy: 100ms, doubling to 800ms, 1.5s in all.
const (
	startRetryFirst = 100 * time.Millisecond
	startRetryLimit = 800 * time.Millisecond
)

// startedSuite is a suite process started under its managed process.
type startedSuite struct {
	cmd            *exec.Cmd
	mp             *ManagedProcess
	stdout, stderr *bytes.Buffer
	cutShort       *atomic.Bool
}

// suiteStart starts a suite process; a variable so tests can count starts.
var suiteStart = startSuite

func startSuite(ctx context.Context, target SuiteTarget, env []string, test2json bool) (startedSuite, error) { //nolint:gocritic // hugeParam: a copy per start is nothing beside the process it starts
	cmd := buildSuiteCmd(ctx, target, env, test2json)
	s := startedSuite{cmd: cmd, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, cutShort: &atomic.Bool{}}
	cmd.Stdout = s.stdout
	cmd.Stderr = s.stderr

	mp := NewManagedProcess(cmd, ProcessConfig{
		Grace:      GraceBudget,
		BudgetFile: target.BudgetFile,
		StopFile:   target.StopFile,
	})
	s.mp = mp
	// exec cancels only a process still running when ctx ends.
	cancel := cmd.Cancel
	cmd.Cancel = func() error {
		s.cutShort.Store(true)
		// Announced before the signal: the process reads it on the signal,
		// or polls for it where no signal comes.
		if target.StopFile != "" {
			_ = os.WriteFile(target.StopFile, nil, 0o600)
			if pid := readPID(protocol.StopPIDPath(target.StopFile)); pid > 0 {
				if err := signalSuite(pid, cmd.Process.Pid); err == nil {
					mp.Stopped()
					return nil
				}
			}
		}
		return cancel()
	}
	return s, mp.Start()
}

// waitRetry waits d, or returns ctx's error once it ends.
func waitRetry(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunSingleSuite executes a single suite subprocess.
// When test2json is true, the binary is wrapped with the test2json tool.
//
// On context cancellation, cmd.Cancel announces the stop and signals the
// suite process alone, by the pid it published on Unix and by the stop file
// itself on Windows, so the converter in front of it relays the stream to its
// end; a process that published no pid, or one outside the suite's process
// group, gets the group's SIGTERM instead. A suite stopped alone leaves
// what it started running; once the root has exited, the rest of the group
// is interrupted and, past the budget, killed. A per-process kill timer (from
// the teardown budget file) governs when the suite itself is killed. Output is
// read by the managed process itself, so a grandchild outside the group that
// keeps the pipes open holds the result for the drain delay at most.
func RunSingleSuite(ctx context.Context, target SuiteTarget, env []string, test2json bool) SuiteResult { //nolint:gocritic // hugeParam: stable API
	start := time.Now()
	s, err := suiteStart(ctx, target, env, test2json)
	for retry := time.Duration(0); errors.Is(err, syscall.ETXTBSY) && retry < startRetryLimit; {
		// Another process still has the executable open for writing: a go
		// command writing a tool into the shared build cache in place.
		// Unix reports it; Windows refuses such a file with a sharing
		// violation instead.
		// It covers starting the suite or test2json, which execs the suite
		// binary itself; copyFile's ForkLock guards the runner's own writes.
		retry = max(2*retry, startRetryFirst)
		if err = waitRetry(ctx, retry); err != nil {
			break // a cancelled start, as exec reports one
		}
		s, err = suiteStart(ctx, target, env, test2json)
	}
	if err != nil {
		return SuiteResult{
			Target:   target,
			Stderr:   fmt.Appendf(nil, "gotest: %s did not start: %v\n", target.SuiteName, err),
			ExitCode: 2,
			Started:  start,
			Duration: time.Since(start),
		}
	}
	cmd, mp, stdout, stderr, cutShort := s.cmd, s.mp, s.stdout, s.stderr, s.cutShort

	_ = mp.WaitWithGrace(ctx)
	duration := time.Since(start)

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	// A failed test exits 1, a panic 2, os.Exit whatever it was given: each
	// is a failed suite, 1, as go test reports it. A status the binary did
	// not choose is left for RecordResult to read.
	if _, how := ExitStatusVerdict(exitCode); how == "" && exitCode > 0 {
		exitCode = 1
	}

	return SuiteResult{
		Target:   target,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
		Started:  start,
		Duration: duration,
		CutShort: cutShort.Load(),
	}
}

func readTeardownBudget(path string) time.Duration {
	if path == "" {
		return GracefulShutdownDelay
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return GracefulShutdownDelay
	}
	d, err := time.ParseDuration(strings.TrimSpace(string(data)))
	if err != nil || d <= 0 {
		return GracefulShutdownDelay
	}
	return d
}

// StripTrailingStatus removes the binary's own PASS or FAIL status line, which
// the runner reprints per package. It is the last such line, not necessarily
// the last line: fixture teardown runs after m.Run printed it and may write.
func StripTrailingStatus(data []byte) []byte {
	lines := bytes.SplitAfter(data, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if line := string(bytes.TrimRight(lines[i], "\n")); line == "PASS" || line == "FAIL" {
			rest := lines[i+1:]
			if len(bytes.TrimSpace(bytes.Join(rest, nil))) == 0 {
				rest = nil // only blank lines followed the status
			}
			return bytes.Join(append(lines[:i:i], rest...), nil)
		}
	}
	return data
}

func WritePackageSummary(pkg string, failed bool, d time.Duration, verbose bool) {
	switch {
	case failed:
		fmt.Fprintf(os.Stdout, "FAIL\nFAIL\t%s\t%.3fs\n", pkg, d.Seconds())
	case verbose:
		fmt.Fprintf(os.Stdout, "PASS\nok  \t%s\t%.3fs\n", pkg, d.Seconds())
	default:
		fmt.Fprintf(os.Stdout, "ok  \t%s\t%.3fs\n", pkg, d.Seconds())
	}
}

// BuildSuiteTargets constructs SuiteTarget entries from compiled binaries
// and suite names. suitesByPkg maps import path to a list of suite struct
// names (e.g., "FooTestSuite"). The generated test function name is
// "Test" + suite struct name. fuzzFuncsByPkg maps import path to a map of
// suite struct name to its generated Fuzz<Suite>_<Method> wrapper names
// (see OverlayResult.FuzzFuncsByPkg); each target's FuzzFuncs is populated
// from its own pkg+suite entry so its seed corpus replays alongside the
// suite's Test function in a plain run (see buildSuiteCmd).
//
// testlessByPkg (import path → suite struct name → true) marks suites with
// no Test function: they run for their fuzz seeds alone.
//
// A suite is included when userRunFilter (-run) enters one of its top-level
// functions and userSkipFilter (-skip) does not skip them all, as the
// testing package decides it.
//
// exclusiveByPkg (import path → suite struct name → true) marks suites with
// SuiteConfig{Exclusive: true}; their targets carry Exclusive and are
// dispatched strictly alone, after every non-exclusive suite.
func BuildSuiteTargets(compiled []CompileResult, suitesByPkg map[string][]string, dirsByPkg map[string]string, fuzzFuncsByPkg map[string]map[string][]string, testlessByPkg map[string]map[string]bool, exclusiveByPkg map[string]map[string]bool, runFlags []string, userRunFilter, userSkipFilter string) []SuiteTarget {
	binByPkg := make(map[string]string, len(compiled))
	for _, cr := range compiled {
		binByPkg[cr.Package] = cr.BinaryPath
	}

	translatedFlags := TranslateToTestBinaryFlags(runFlags)

	var targets []SuiteTarget
	for pkg, suites := range suitesByPkg {
		bin, ok := binByPkg[pkg]
		if !ok {
			continue
		}

		pkgDir := dirsByPkg[pkg]

		for _, suiteName := range suites {
			testFuncName := "Test" + suiteName
			testless := testlessByPkg[pkg][suiteName]
			funcs := suiteFuncs(suiteName, testless, fuzzFuncsByPkg[pkg][suiteName])
			if !selectsSuite(userRunFilter, funcs) || skipsSuite(userSkipFilter, funcs) {
				continue
			}
			target := SuiteTarget{
				SuiteSpec: SuiteSpec{
					Package:   pkg,
					Dir:       pkgDir,
					SuiteName: testFuncName,
				},
				BinaryPath: bin,
				RunFlags:   translatedFlags,
				FuzzFuncs:  fuzzFuncsByPkg[pkg][suiteName],
				Exclusive:  exclusiveByPkg[pkg][suiteName],
			}
			if rf := suiteRunFilter(userRunFilter, testFuncName); rf != "" {
				target.RunFilter = rf
			} else if userRunFilter != "" && (testless || !selectsFunc(userRunFilter, testFuncName)) {
				// Selected through a fuzz wrapper's name alone: the user's
				// filter is the whole -test.run, so only that wrapper runs.
				target.RunFilter = userRunFilter
			}
			targets = append(targets, target)
		}
	}
	return targets
}

// suiteFuncs lists the top-level functions of a suite's test run: its Test
// function unless it has none, and its generated fuzz wrappers, the names -v
// prints for seed replays.
func suiteFuncs(suiteName string, testless bool, fuzzFuncs []string) []string {
	if testless {
		return fuzzFuncs
	}
	return append([]string{"Test" + suiteName}, fuzzFuncs...)
}

// selectsSuite reports whether the user's -run filter enters one of funcs.
func selectsSuite(userRunFilter string, funcs []string) bool {
	if userRunFilter == "" {
		return true
	}
	return slices.ContainsFunc(funcs, func(f string) bool { return selectsFunc(userRunFilter, f) })
}

// skipsSuite reports whether the user's -skip filter drops a suite whole: go
// test would enter none of funcs. Otherwise the suite runs and the binary
// applies the filter.
func skipsSuite(userSkipFilter string, funcs []string) bool {
	if userSkipFilter == "" || len(funcs) == 0 {
		return false
	}
	return !slices.ContainsFunc(funcs, func(f string) bool { return !skipsFunc(userSkipFilter, f) })
}

// BuildBenchTargets constructs SuiteTarget entries for benchmark suites from
// compiled binaries and bench-eligible suite names. benchesByPkg maps import
// path to a list of suite struct names (e.g., "FooTestSuite") that have at
// least one effective benchmark. The generated benchmark wrapper function
// name is "Benchmark" + suite struct name.
//
// userRunFilter (from -run) and userBenchFilter (from -bench) are applied
// independently, each matched against the benchmark wrapper name by its
// top level, as testing matches it. When both are non-empty, a suite must
// satisfy both (AND semantics) to be included; either one alone filters on
// its own, and when both are empty all suites are included. userSkipFilter
// (from -skip) drops a suite whose wrapper it skips whole; the binary still
// receives it and skips the sub-benchmarks it names.
//
// A userBenchFilter with sub-benchmark segments additionally scopes what
// runs inside the wrapper: the per-suite portion of the pattern (extracted
// with suiteRunFilter, the same helper -run uses for subtest scoping) is
// carried on the target as BenchFilter and becomes the binary's -test.bench
// value, where go test's slash matching selects the b.Run sub-benchmarks.
func BuildBenchTargets(compiled []CompileResult, benchesByPkg map[string][]string, dirsByPkg map[string]string, runFlags []string, userRunFilter, userBenchFilter, userSkipFilter string) []SuiteTarget {
	binByPkg := make(map[string]string, len(compiled))
	for _, cr := range compiled {
		binByPkg[cr.Package] = cr.BinaryPath
	}

	translatedFlags := TranslateToTestBinaryFlags(runFlags)

	var targets []SuiteTarget
	for pkg, suites := range benchesByPkg {
		bin, ok := binByPkg[pkg]
		if !ok {
			continue
		}

		pkgDir := dirsByPkg[pkg]

		for _, suiteName := range suites {
			benchFuncName := "Benchmark" + suiteName
			if !selectsBench(userRunFilter, userBenchFilter, userSkipFilter, benchFuncName) {
				continue
			}
			target := SuiteTarget{
				SuiteSpec: SuiteSpec{
					Package:   pkg,
					Dir:       pkgDir,
					SuiteName: suiteName,
				},
				BinaryPath:  bin,
				RunFlags:    translatedFlags,
				Bench:       true,
				BenchFilter: suiteRunFilter(userBenchFilter, benchFuncName),
			}
			targets = append(targets, target)
		}
	}
	return targets
}

// sortTargetIndices orders target indices by (Package, SuiteName) so
// exclusive dispatch is deterministic run over run.
func sortTargetIndices(targets []SuiteTarget, idx []int) {
	sort.Slice(idx, func(a, b int) bool {
		ta, tb := targets[idx[a]], targets[idx[b]]
		if ta.Package != tb.Package {
			return ta.Package < tb.Package
		}
		return ta.SuiteName < tb.SuiteName
	})
}

// quoteAll returns regexp.QuoteMeta applied to each name, preserving order.
func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = regexp.QuoteMeta(n)
	}
	return out
}

// suiteRunFilter extracts the portion of userRunFilter that applies to
// testFuncName and includes a subtest component. Returns empty string when
// the filter has no subtest part (suite-name-only matching suffices).
func suiteRunFilter(userRunFilter, testFuncName string) string {
	if userRunFilter == "" || !strings.Contains(userRunFilter, "/") {
		return ""
	}

	alts := splitTopLevelOr(userRunFilter)
	if len(alts) == 1 {
		return userRunFilter
	}

	var matching []string
	for _, alt := range alts {
		if selectsFunc(alt, testFuncName) {
			matching = append(matching, alt)
		}
	}
	if len(matching) == 0 {
		return ""
	}
	return strings.Join(matching, "|")
}

// splitTopLevelOr splits a pattern into its alternatives as the testing
// package does.
func splitTopLevelOr(pattern string) []string { return splitTopLevel(pattern, '|') }

// insertBeforeWords adds a flag ahead of the binary's own words, which the
// translated -args left in place: flag parsing stops at the first word that
// is not a flag.
func insertBeforeWords(args []string, flag string) []string {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			return slices.Insert(slices.Clone(args), i, flag)
		}
	}
	return append(slices.Clone(args), flag)
}

// readPID reads the pid a suite process published, 0 when there is none.
func readPID(path string) int {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the runner's own sidecar file
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}
