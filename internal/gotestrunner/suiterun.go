package gotestrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/control"
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
	Bench        bool     // when true, run the Benchmark<SuiteName> wrapper instead of the suite's tests
	BenchFilter  string   // raw -test.bench value carrying the user's sub-benchmark segments (empty = the exact Benchmark<SuiteName> wrapper)
	Exclusive    bool     // SuiteConfig{Exclusive: true}: dispatched strictly alone, after every non-exclusive suite of its group
	Group        string   // SuiteConfig.ExclusiveGroup: the scope the suite runs in, "" for the default group
	FuzzFuncs    []string // generated Fuzz<Suite>_<Method> wrapper names whose seed corpus replays alongside SuiteName in a plain run (ignored when Bench is true or RunFilter is set)
	Exec         []string // go test's -exec: the command the binary runs under (empty = the binary itself)
	CountsDir    string   // where a text run's process writes its leaves' assertion counts (empty = it writes none)
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
	// DeadlineCut reports that the process was cut short and the run's
	// context ended by its deadline: the suite did not finish.
	DeadlineCut bool
	// StoppedBy is the reason the suite process gave for a stop the runner
	// did not send, such as "signal:SIGTERM"; empty when there was none.
	StoppedBy string
	// Counts holds the protocol.LeafCount lines a text run's process wrote;
	// nil when none was asked for or none was written.
	Counts []byte
	// relayed is the census the process's events went through as they were
	// written; nil for events that wait in Stdout.
	relayed *suiteCensus
}

// RunMode controls how a run executes its suites and collects their output.
type RunMode int

const (
	// RunBatchText runs suites with no converter, their stdout and stderr
	// apart. Output is batched per-package: when all suites for a package
	// complete, their output is written to stdout followed by a package
	// summary line.
	RunBatchText RunMode = iota

	// RunStreamJSON converts each suite's output to JSON events with
	// test2json beside it (see relay) and streams them to stdout as soon as
	// the suite completes.
	RunStreamJSON

	// RunCaptureJSON converts each suite's output as RunStreamJSON does and
	// captures all JSON events into a buffer (nothing written to stdout).
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
	}) //nolint:gocritic // mirror of sortDeferred over the targets themselves
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
			runRecorded(ctx, collector, targets[i].Package, localIdx[i], targets[i], env, useTest2JSON)
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
		// The user's -run narrowed to this process's own functions (see
		// processRunFilter): another suite's Test function never runs here,
		// and fuzz seeds replay only for a wrapper the pattern names.
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

	argv := append(slices.Clone(target.Exec), target.BinaryPath)
	argv = append(argv, testArgs...)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: binary built by this tool, under the -exec command the user named
	cmd.Env = slices.Clone(env)
	cmd.Dir = target.Dir
	return cmd
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
	relay          *relay // the converter beside a suite on a JSON pipeline
}

// attachChannel hands a suite process its end of the channel; a variable so
// tests can start one that never takes it.
var attachChannel = (*control.ChildEnd).Attach

// suiteStart starts a suite process; a variable so tests can count starts.
var suiteStart = startSuite

// startSuite starts a suite process; live, when not nil, relays its events
// as the converter writes them.
func startSuite(ctx context.Context, target SuiteTarget, env []string, test2json bool, live *liveSuite) (startedSuite, error) { //nolint:gocritic // hugeParam: a copy per start is nothing beside the process it starts
	cmd := buildSuiteCmd(ctx, target, env, test2json)
	s := startedSuite{cmd: cmd, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, cutShort: &atomic.Bool{}}
	link, child, err := control.New()
	if err != nil {
		return s, err
	}
	attachChannel(child, cmd)
	if test2json {
		// One writer for both streams gets one pipe, which keeps their order.
		s.relay = startRelay(target, s.stdout, s.stderr, live)
		cmd.Stdout, cmd.Stderr = s.relay, s.relay
	} else {
		cmd.Stdout, cmd.Stderr = s.stdout, s.stderr
	}

	mp := NewManagedProcess(cmd, ProcessConfig{
		Grace:       GraceBudget,
		Link:        link,
		HoldJob:     true,
		AwaitOutput: test2json && len(target.Exec) > 0,
	})
	s.mp = mp
	// exec cancels only a process still running when ctx ends.
	cmd.Cancel = func() error {
		s.cutShort.Store(true)
		return mp.RequestStop()
	}
	err = mp.Start()
	child.Close()
	if err != nil {
		if s.relay != nil {
			s.relay.discard()
		}
		return s, err
	}
	return s, nil
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

// RunSingleSuite executes a single suite subprocess: the binary, behind the
// -exec command if there is one, as the root of its own process tree. When
// test2json is true its stdout and stderr are one stream, which a converter
// beside it turns into JSON events (see relay); the result carries the
// suite's own exit status either way.
//
// On context cancellation, cmd.Cancel sends the suite process the stop line
// on its channel, or, when it took none within helloWithin of its start or
// the line cannot be written, a Ctrl-C to its tree. A suite stopped by the
// line leaves what it started running; once the root has exited, the rest of
// the group is interrupted and, past the budget, killed. The teardown budget
// the process reports on its channel governs when the suite itself is
// killed. Output is read by the managed process itself, so a grandchild
// outside the group that keeps the pipes open holds the result for the drain
// delay at most.
func RunSingleSuite(ctx context.Context, target SuiteTarget, env []string, test2json bool) SuiteResult { //nolint:gocritic // hugeParam: stable API
	return runSuiteProcess(ctx, target, env, test2json, nil)
}

// runSuiteProcess is RunSingleSuite, its events relayed through live as they
// are written when live is not nil.
func runSuiteProcess(ctx context.Context, target SuiteTarget, env []string, test2json bool, live *liveSuite) SuiteResult { //nolint:gocritic // hugeParam: as RunSingleSuite
	counts, env := askCounts(target, env, test2json)
	defer removeCounts(counts)
	start := time.Now()
	s, err := suiteStart(ctx, target, env, test2json, live)
	for retry := time.Duration(0); errors.Is(err, syscall.ETXTBSY) && retry < startRetryLimit; {
		// Another process still has the executable open for writing: a go
		// command writing a tool into the shared build cache in place.
		// Unix reports it; Windows refuses such a file with a sharing
		// violation instead.
		// It covers the binary the runner starts itself, not one an -exec
		// command starts; copyFile's ForkLock guards the runner's own writes.
		retry = max(2*retry, startRetryFirst)
		if err = waitRetry(ctx, retry); err != nil {
			break // a cancelled start, as exec reports one
		}
		s, err = suiteStart(ctx, target, env, test2json, live)
	}
	if err != nil {
		return SuiteResult{
			Target:   target,
			Stderr:   fmt.Appendf(nil, "gotest: %s did not start: %v\n%s", target.SuiteName, err, foreignTargetNote(goSettings(), runtime.GOOS, runtime.GOARCH, target.Exec)),
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
	if s.relay != nil && s.relay.finish(exitCode) != nil && exitCode == 0 {
		exitCode = 1 // no events to read a verdict from
	}
	// A failed test exits 1, a panic 2, os.Exit whatever it was given: each
	// is a failed suite, 1, as go test reports it. A status the binary did
	// not choose is left for RecordResult to read.
	if _, how := ExitStatusVerdict(exitCode); how == "" && exitCode > 0 {
		exitCode = 1
	}

	// A method the stop ended may still return and pass: the suite did not
	// finish.
	deadlineCut := cutShort.Load() && errors.Is(ctx.Err(), context.DeadlineExceeded)
	return SuiteResult{
		Target:      target,
		Stdout:      stdout.Bytes(),
		Stderr:      stderr.Bytes(),
		ExitCode:    exitCode,
		Started:     start,
		Duration:    duration,
		CutShort:    cutShort.Load(),
		DeadlineCut: deadlineCut,
		StoppedBy:   mp.StoppedBy(),
		Counts:      readCounts(counts),
	}
}

// askCounts creates the file a text run's suite process writes its leaves'
// counts to and names it in env; "" and env as given for a process that
// writes none. testing prints the counts' attributes only under -v.
func askCounts(target SuiteTarget, env []string, test2json bool) (string, []string) { //nolint:gocritic // hugeParam: as RunSingleSuite
	if target.CountsDir == "" || test2json || target.Bench {
		return "", env
	}
	f, err := os.CreateTemp(target.CountsDir, "*.jsonl")
	if err != nil {
		return "", env
	}
	_ = f.Close()
	return f.Name(), append(slices.Clone(env), protocol.EnvAssertionsFile+"="+f.Name())
}

// readCounts is what the process wrote to path, nil for none.
func readCounts(path string) []byte {
	if path == "" {
		return nil
	}
	data, _ := os.ReadFile(path) //nolint:gosec // G304: the file askCounts created
	return data
}

func removeCounts(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
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

// suiteTargets builds the target of each planned suite of one compiled
// package. suites are the package's share of a test run's selection, so no
// selection is applied here. Each target runs the suite's Test function, its
// fuzz wrappers' seeds beside it (see buildSuiteCmd), and the part of the
// user's -run that enters its own top-level functions.
func suiteTargets(bin CompileResult, suites []*PlannedSuite, runFlags []string, userRunFilter string) []SuiteTarget { //nolint:gocritic // hugeParam: once per package
	translatedFlags := TranslateToTestBinaryFlags(runFlags)
	targets := make([]SuiteTarget, 0, len(suites))
	for _, s := range suites {
		funcs := suiteFuncs(s.Name, s.Testless, s.FuzzNames())
		targets = append(targets, SuiteTarget{
			SuiteSpec: SuiteSpec{
				Package:   s.Package,
				Dir:       s.Dir,
				SuiteName: s.TestFunc(),
				RunFilter: processRunFilter(userRunFilter, funcs),
			},
			BinaryPath: bin.BinaryPath,
			RunFlags:   translatedFlags,
			FuzzFuncs:  s.FuzzNames(),
			Exclusive:  s.Exclusive,
			Group:      s.Group,
		})
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

// benchTargets builds the bench target of each planned suite whose package
// compiled. suites are a bench run's selection (see benchRun): -run, -bench
// and -skip are matched against Benchmark<Suite> there.
//
// A userBenchFilter with sub-benchmark segments additionally scopes what
// runs inside the wrapper: the per-suite portion of the pattern (extracted
// with processRunFilter, the same helper -run uses) is carried on the target
// as BenchFilter and becomes the binary's -test.bench value, where go test's
// slash matching selects the b.Run sub-benchmarks.
func benchTargets(compiled []CompileResult, suites []*PlannedSuite, runFlags []string, userBenchFilter string) []SuiteTarget {
	binByPkg := make(map[string]string, len(compiled))
	for _, cr := range compiled {
		binByPkg[cr.Package] = cr.BinaryPath
	}
	translatedFlags := TranslateToTestBinaryFlags(runFlags)
	var targets []SuiteTarget
	for _, s := range suites {
		bin, ok := binByPkg[s.Package]
		if !ok {
			continue
		}
		targets = append(targets, SuiteTarget{
			SuiteSpec: SuiteSpec{
				Package:   s.Package,
				Dir:       s.Dir,
				SuiteName: s.Name,
			},
			BinaryPath:  bin,
			RunFlags:    translatedFlags,
			Bench:       true,
			BenchFilter: processRunFilter(userBenchFilter, []string{s.BenchFunc()}),
		})
	}
	return targets
}

// quoteAll returns regexp.QuoteMeta applied to each name, preserving order.
func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = regexp.QuoteMeta(n)
	}
	return out
}

// processRunFilter narrows a user -run (or -bench) pattern to one suite
// process's own top-level functions: every alternative whose first level
// matches one of funcs is kept, its first level replaced by that function's
// exact name and its later levels as written, so no other suite's function
// runs in the process. Empty for no pattern; a pattern that does not compile
// is passed whole for the binary to report.
func processRunFilter(userFilter string, funcs []string) string {
	if userFilter == "" {
		return ""
	}
	if _, err := parseNameFilter(userFilter, "-run"); err != nil {
		return userFilter
	}
	alts, _ := splitNameFilter(userFilter)
	var out []string
	for _, elems := range alts {
		re := regexp.MustCompile(rewriteName(elems[0]))
		for _, f := range funcs {
			if !re.MatchString(f) {
				continue
			}
			alt := strings.Join(append([]string{"^" + regexp.QuoteMeta(f) + "$"}, elems[1:]...), "/")
			if !slices.Contains(out, alt) {
				out = append(out, alt)
			}
		}
	}
	return strings.Join(out, "|")
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
