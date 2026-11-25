package gotestrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/schedinfo"
)

// resolveSetupTimeout turns the --setup-timeout flag into WaitSetup's flat
// budget: a positive duration as given, zero (the flag absent) for the
// fixtures' declared budgets, negative (the flag at 0) for no deadline.
func resolveSetupTimeout(d time.Duration) time.Duration {
	switch {
	case d > 0:
		return d
	case d < 0:
		return -1
	default:
		return 0
	}
}

// computeDispatchConcurrency decides how many suite processes run at once.
// The budget (total concurrent test methods, default 2×GOMAXPROCS) is split
// between processes and each process's -parallel; a user -parallel keeps the
// budget and divides it, so processes × methods never exceeds the budget.
// procCap (go test's -p) sets the process count, above gomaxprocs too and
// without the sanitizer halving: as go test, it runs that many binaries.
func computeDispatchConcurrency(runFlags *[]string, budget, totalSuites, procCap int, sanitized bool, gomaxprocs int) int {
	userParallel := ExtractParallelValue(*runFlags)

	procs := gomaxprocs
	if sanitized && budget == 0 && procCap == 0 {
		// Halve both dimensions: the budget (total concurrent test methods)
		// and the process cap. Halving the budget alone would not reduce the
		// process count — ComputeConcurrency caps inter at procs first — and
		// the OS process running an instrumented binary is the costly unit.
		budget = procs
		procs = max(1, procs/2)
	}
	if procCap > 0 {
		procs = procCap
	}
	if userParallel > 0 {
		if budget <= 0 {
			budget = 2 * procs
		}
		return max(1, min(totalSuites, procs, budget/userParallel))
	}
	inter, intra := ComputeConcurrency(budget, totalSuites, procs)
	*runFlags = InjectParallel(*runFlags, intra)
	return inter
}

type PipelineConfig struct {
	GoTestArgs      []string
	SetupTimeout    time.Duration
	UpdateSnapshots bool
	CI              bool
	Parallel        int
	CompileParallel int
	OutputMode      RunMode
	Bench           bool
	// GlobalTimeout names the deadline in the failure of a run that outlives
	// it; the deadline itself arrives on ctx.
	GlobalTimeout time.Duration
	// NoTestFiles are matched packages without test files; like those
	// without suites, each is reported as go test reports it.
	NoTestFiles []string
	// PackageOrder is every matched package in the order go test reports
	// them; a text run prints its verdicts in that order.
	PackageOrder []string
	// Events receives a copy of the run's go test -json event stream as it
	// is written; nil for none.
	Events io.Writer
	// Groups and SkipGroups are --group and --skip-group: the named groups
	// the run is narrowed to, and those it leaves out.
	Groups, SkipGroups []string
	// Manifest opens a -json stream, which EndRun then closes; nil for a
	// stream that is neither.
	Manifest *protocol.Manifest
	// Only narrows the run as --failed does: each package's own -run, ""
	// for all its suites; a package it lacks runs nothing. Nil for none.
	Only map[string]string
	// Stdout is where a JSON stream goes; nil for os.Stdout.
	Stdout io.Writer
	// Inputs is each package's input hash, which stamps its verdict; nil
	// for none.
	Inputs func(pkg string) string
}

// flushOrder is the order a text run prints its verdicts in: the matched
// packages as go test orders them, else broken, suite and suiteless ones.
func flushOrder(cfg PipelineConfig, overlay *OverlayResult, noSuite []string) []string { //nolint:gocritic // hugeParam: stable API
	if len(cfg.PackageOrder) > 0 {
		return cfg.PackageOrder
	}
	order := make([]string, 0, len(overlay.BrokenPackages)+len(overlay.SuitePackages)+len(noSuite))
	for i := range overlay.BrokenPackages {
		order = append(order, overlay.BrokenPackages[i].PkgPath)
	}
	return append(append(order, overlay.SuitePackages...), noSuite...)
}

// bookNoSuites books the packages without suites, in report order. Their
// verdicts are known before anything runs, so the stream gets them first.
func bookNoSuites(c *OutputCollector, overlay *OverlayResult, noSuite, order []string) {
	rank := make(map[string]int, len(order))
	for i, pkg := range order {
		rank[pkg] = i
	}
	noSuite = slices.Clone(noSuite)
	slices.SortStableFunc(noSuite, func(a, b string) int { return rank[a] - rank[b] })
	skipped := overlay.plan().SkippedSuites()
	for _, pkg := range noSuite {
		c.RecordNoSuite(pkg, len(skipped[pkg]) > 0)
	}
}

type PipelineResult struct {
	ExitCode     int
	CapturedJSON []byte
	// Groups is what each named group that ran took, by name.
	Groups []GroupRun
	// CompileSpan runs from the compile stage's start to its last outcome;
	// zero when nothing was compiled.
	CompileSpan time.Duration
	// Judged is what the run judged, its exit code aside; nil for a run that
	// judged nothing, such as -list. A text run counts no tests.
	Judged *protocol.RunResult
	// Failed are the units the run failed, for --failed.
	Failed []protocol.FailedUnit
	// Packages are the verdicts of the packages the run judged.
	Packages []protocol.PackageVerdict
}

// sharedFixturesPkg names the synthetic package a shared fixture failure is
// booked under. The space keeps it out of the import-path namespace.
const sharedFixturesPkg = "shared fixtures"

// fixtureFailure is a shared fixture failure together with what the fixtures
// reported about it on stderr.
type fixtureFailure struct {
	err     error
	reports []string
}

func (f *fixtureFailure) Error() string { return f.err.Error() }
func (f *fixtureFailure) Unwrap() error { return f.err }

// withReports attaches to err the reports that are about its phase: a
// teardown's are the AfterAll lines, a setup's are the others.
func withReports(err error, reports []string, teardown bool) error {
	if err == nil {
		return nil
	}
	var own []string
	for _, r := range reports {
		if strings.Contains(r, ".AfterAll ") == teardown {
			own = append(own, r)
		}
	}
	if len(own) == 0 {
		return err
	}
	return &fixtureFailure{err: err, reports: own}
}

// teardownVerdict is the teardown's error with the fixtures' AfterAll reports
// attached. The reports are stderr, free text a fixture can write as well:
// they explain a failure, they never make one.
func teardownVerdict(teardownErr error, reports []string) error {
	return withReports(teardownErr, reports, true)
}

// applyTeardownFailure surfaces a shared fixture teardown failure and makes a
// run that would otherwise have passed fail instead. Resources the fixtures
// hold outlive the test process, so leaving them behind must not report ok.
func applyTeardownFailure(c *OutputCollector, result *PipelineResult, err error) {
	applyFixtureFailure(c, result, err)
}

// applySetupFailure fails a run whose shared fixtures did not come up. The
// suites that read them never ran, so this is a failed run (1) and not a
// command that broke (2), in every mode.
func applySetupFailure(c *OutputCollector, result *PipelineResult, err error) {
	applyFixtureFailure(c, result, err)
}

func applyFixtureFailure(c *OutputCollector, result *PipelineResult, err error) {
	if err == nil {
		return
	}
	var reports []string
	if f := (*fixtureFailure)(nil); errors.As(err, &f) {
		reports = f.reports
	}
	failRun(c, result, sharedFixturesPkg, err.Error(), reports...)
}

// applyDeadlineFailure fails a run whose deadline expired before its last
// verdict, naming the units it cut short. With none to name, the failure is
// booked as a synthetic package so the reason still reaches every renderer.
// waiting names the scope a wait for another run was cut short for, "" for none.
func applyDeadlineFailure(c *OutputCollector, result *PipelineResult, cfg PipelineConfig, dispatchErr error, running []CensusCase, waiting string) { //nolint:gocritic // hugeParam: stable API
	if !errors.Is(dispatchErr, context.DeadlineExceeded) {
		return
	}
	msg := "run deadline exceeded"
	if cfg.GlobalTimeout > 0 {
		msg = fmt.Sprintf("global --timeout exceeded after %v", cfg.GlobalTimeout)
	}
	if waiting != "" {
		msg += " while " + waiting + " waited for another gotest run"
	}
	if len(running) == 0 {
		// No method was left running: name the suites the deadline stopped.
		if cut := c.cutShortUnits(); len(cut) > 0 {
			failRun(c, result, globalTimeoutPkg, msg+" while running: "+unitNames(cut))
			return
		}
		failRun(c, result, globalTimeoutPkg, msg)
		return
	}
	c.reportFailure("FAIL: %s while running: %s\n", msg, unitNames(running))
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
}

// failRun reports msg and fails a run that would otherwise pass. The stream is
// what every renderer and -json consumer derives from, so the failure is booked
// into it through the collector as a failed synthetic package, in the live and
// the captured mode alike, instead of living on the exit code alone. details
// are lines stderr has carried already; only the stream still needs them.
func failRun(c *OutputCollector, result *PipelineResult, pkg, msg string, details ...string) {
	c.reportFailure("FAIL: %s\n", msg)
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
	c.bookRunFailure(pkg, msg, details...)
}

// stageFailurePkg names the synthetic package under which a build failure
// that belongs to no single package (e.g. the compile stage's own setup) is
// booked. The space keeps it out of the import-path namespace.
const stageFailurePkg = "go build"

// globalTimeoutPkg names the synthetic package an expired run deadline is
// booked under when no unit was running to name.
const globalTimeoutPkg = "global --timeout"

// brokenPackageMessage renders a load-broken package's diagnostics in the
// `go build` shape: a `# path` header followed by one diagnostic per line.
// The compiler's own report brings its header along.
func brokenPackageMessage(bp *gotestgen.BrokenPackage) string {
	var b strings.Builder
	if len(bp.Errors) == 0 || !strings.HasPrefix(bp.Errors[0], "# ") {
		fmt.Fprintf(&b, "# %s\n", bp.PkgPath)
	}
	for _, e := range bp.Errors {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	return b.String()
}

// bookBuildFailures books load-broken packages and per-package compile
// failures into the collector. Both are package verdicts: they must reach the
// rendered output, the JSON stream, and the exit code through the same
// collector every real suite result flows through.
func bookBuildFailures(c *OutputCollector, broken []gotestgen.BrokenPackage, failures []BuildFailure) {
	for i := range broken {
		c.RecordBuildFailure(broken[i].PkgPath, brokenPackageMessage(&broken[i]))
	}
	for _, f := range failures {
		pkg := f.Package
		if pkg == "" {
			pkg = stageFailurePkg
			c.markRunLevel(pkg)
		}
		c.RecordBuildFailure(pkg, f.Err.Error()+"\n")
	}
}

// runFailureEvents renders test2json-shaped events recording a run-level
// failure that happened outside any test binary, after its stream ended.
func runFailureEvents(pkg, msg string, details ...string) []byte {
	var stream []byte
	ev := func(action, text string) []byte {
		e := struct {
			Action   string `json:"Action"`
			Package  string `json:"Package"`
			Output   string `json:"Output,omitempty"`
			RunLevel bool   `json:"RunLevel"`
		}{Action: action, Package: pkg, Output: text, RunLevel: true}
		b, _ := json.Marshal(e)
		return append(b, '\n')
	}
	stream = append(stream, ev("start", "")...)
	stream = append(stream, ev("output", "FAIL: "+msg+"\n")...)
	for _, d := range details {
		stream = append(stream, ev("output", d+"\n")...)
	}
	stream = append(stream, ev("fail", "")...)
	return stream
}

func RunPipeline(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult) (PipelineResult, error) { //nolint:gocritic // hugeParam: stable API
	if !cfg.CI && os.Getenv(protocol.EnvCI) == "" {
		if protocol.SwitchOn(os.Getenv("CI")) {
			cfg.CI = true
		}
	}
	if err := overlay.selectGroups(cfg.Groups, cfg.SkipGroups); err != nil {
		return PipelineResult{}, err
	}
	pf := ParseExecFlags(cfg.GoTestArgs)

	// Only when seed corpora actually replay in this run: a stale entry is
	// what would fail, and the engine's own error names the wrapper, not the
	// field that moved.
	if !cfg.Bench && overlay.plan().HasFuzz() {
		ReportStaleFuzzCorpora(os.Stderr, overlay)
	}

	// A bench run compiles everything before it dispatches, so no build
	// competes with a running benchmark. Every other run overlaps the two.
	if !cfg.Bench && extractFlag(pf.RunFlags, "-list") != "" {
		return runList(ctx, cfg, overlay, pf)
	}
	if cfg.Bench {
		return runBench(ctx, cfg, overlay, pf)
	}
	if name := sharedProfileFlag(pf.RunFlags); name != "" {
		sel := selection{Run: pf.UserRunFilter, Skip: pf.UserSkipFilter, Groups: overlay.groups, Only: cfg.Only}
		if n := plannedSuiteCount(planSuiteScopes(overlay.plan(), sel)); n > 1 {
			return PipelineResult{}, fmt.Errorf("%s writes one file, and this run starts %d suite processes that would each write it: select one suite of one package with -run '^Test<Suite>$'", name, n)
		}
	}
	return runStreaming(ctx, cfg, overlay, pf)
}

func buildBaseEnv(cfg PipelineConfig) []string { //nolint:gocritic // hugeParam: stable API
	// A file an outer run asked for is not this run's; askCounts names its own.
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, protocol.EnvAssertionsFile+"=") })
	if cfg.UpdateSnapshots {
		env = append(env, protocol.EnvUpdateSnapshots+"=1")
	}
	if cfg.CI {
		env = append(env, protocol.EnvCI+"=1")
	}
	return env
}

// benchPrep is what a bench run has when its compile and fixture setup are
// both over: every binary that built, every package that did not, and the
// fixture process with whatever came up. A fixture that failed to set up is
// not a run abort: the targets that read it are given up, the rest run.
type benchPrep struct {
	compiled        []CompileResult
	compileFailures []BuildFailure
	proc            *SharedFixtureProcess
	setupErr        error
	setupTimedOut   bool
	compileSpan     time.Duration
}

// prepareBenchRun compiles packages and starts the given shared fixtures
// concurrently, and waits for both: a bench run dispatches nothing while a
// build or a setup could compete with a measurement. fixtures is the run's
// residency plan (see planBenchFixtureWindows), not the full overlay set: a
// fixture no scheduled target requires never starts.
func prepareBenchRun(ctx context.Context, overlay *OverlayResult, packages []string, fixtures []gotestgen.SharedFixtureInfo, buildFlags, runFlags []string, setupTimeout time.Duration, compileParallel int) benchPrep {
	var p benchPrep
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		link := stageFlags(buildFlags, runFlags, overlay.WorkDir)
		ch, span := spanOf(CompilePackagesStream(ctx, packages, overlay.OverlayFlag, link, overlay.WorkDir, binaryCacheDir(overlay.BinaryCacheRoot, link), compileParallel), time.Now)
		p.compiled, p.compileFailures = collectOutcomes(ch)
		p.compileSpan = span()
	}()

	if len(fixtures) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proc, err := StartSharedFixtures(ctx, overlay.WorkDir, fixtures, setupTimeout, buildFlags...)
			if err != nil {
				p.setupErr = fmt.Errorf("shared fixture setup failed: %w", err)
				return
			}
			p.proc = proc
			var timedOut *SetupTimeoutError
			switch err := proc.WaitSetup(ctx, setupTimeout); {
			case err == nil, ctx.Err() != nil:
			case errors.As(err, &timedOut):
				// Scheduling context: a setup deadline is a wall-clock
				// verdict, and a starved setup looks exactly like a hung one.
				p.setupTimedOut = true
				p.setupErr = fmt.Errorf("%w %s", err, schedinfo.Summary())
			default:
				p.setupErr = err
			}
		}()
	}

	wg.Wait()
	return p
}

func assignCoverProfiles(targets []SuiteTarget, coverDir string) {
	for i := range targets {
		targets[i].CoverProfile = filepath.Join(coverDir, fmt.Sprintf("%d.out", i))
	}
}

func mergeCoverProfiles(targets []SuiteTarget, userProfile string) {
	var profiles []string
	for i := range targets {
		if targets[i].CoverProfile != "" {
			profiles = append(profiles, targets[i].CoverProfile)
		}
	}
	if len(profiles) == 0 {
		return
	}
	if err := MergeCoverProfiles(profiles, userProfile); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: merge cover profiles: %s\n", err)
	}
}

func setupCoverage(targets []SuiteTarget, overlay *OverlayResult, userCoverProfile string) {
	if userCoverProfile == "" {
		return
	}
	coverDir := filepath.Join(overlay.WorkDir, "cover")
	_ = os.MkdirAll(coverDir, 0o755)
	assignCoverProfiles(targets, coverDir)
}

func runBench(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (PipelineResult, error) { //nolint:gocritic // hugeParam: stable API
	// Bench runs dispatch bench targets, not test suites, and match -run and
	// -bench against Benchmark<Suite>: their residency plan must come from
	// the same selection, or fixtures would follow the wrong schedule.
	plan := overlay.plan()
	sel := selection{Run: pf.UserRunFilter, Skip: pf.UserSkipFilter, Bench: ExtractBenchFilter(pf.RunFlags), Groups: overlay.groups}
	win := planBenchFixtureWindows(plan, overlay.SharedFixtures, sel)
	win.reportSkipped()
	collector := NewOutputCollector(cfg.OutputMode, pf.Verbose, WithManifest(cfg.Manifest))
	collector.StdlibTestsByPkg = plan.StdlibTests()
	collector.bench = true
	resolvedSetupTimeout := resolveSetupTimeout(cfg.SetupTimeout)
	// A package no selected benchmark belongs to is not built.
	benchBuild := buildSet(overlay.SuitePackages, dispatchedPackages(plan.benchRun(sel)))
	prep := prepareBenchRun(ctx, overlay, benchBuild, win.Fixtures, pf.BuildFlags, pf.RunFlags, resolvedSetupTimeout, cfg.CompileParallel)
	setupProc, setupErr := prep.proc, prep.setupErr

	// finish tears the fixture process down — the one owner of its
	// shutdown, after the last verdict — and books what the run has to say
	// about its fixtures on top of the verdict handed in.
	finish := func(result PipelineResult, barrierErr error) PipelineResult {
		var teardownErr error
		if setupProc != nil {
			teardownErr = errors.Join(barrierErr, setupProc.Teardown())
			reports := setupProc.TakeFailures()
			setupErr = withReports(setupErr, reports, false)
			teardownErr = teardownVerdict(teardownErr, reports)
		}
		if setupErr != nil && result.ExitCode == 0 {
			result.ExitCode = 1
		}
		applySetupFailure(collector, &result, setupErr)
		applyTeardownFailure(collector, &result, teardownErr)
		result.CapturedJSON = collector.CapturedJSON()
		result.CompileSpan = prep.compileSpan
		return result
	}

	if prepareErr := ctx.Err(); prepareErr != nil {
		result := PipelineResult{ExitCode: exitCodeAfterDispatch(0, prepareErr)}
		applyDeadlineFailure(collector, &result, cfg, prepareErr, nil, "")
		return finish(result, nil), nil
	}

	baseEnv := buildBaseEnv(cfg)

	runFlags := pf.RunFlags
	if cfg.OutputMode == RunCaptureJSON {
		runFlags = insertBeforeArgs(runFlags, "-v")
	}

	// A user-supplied -bench value arrives in RunFlags via normal flag
	// classification. It must not reach buildSuiteCmd as a raw -test.bench
	// flag — that would be appended after the generated
	// -test.bench=^Benchmark<Suite>$ and silently win, defeating per-suite
	// scoping. Extract and strip it here; the bench run's selection matches
	// it against Benchmark<Suite> as a bench-name filter. -run
	// (pf.UserRunFilter) is passed through alongside it as an independent
	// suite filter — both compose with AND semantics, e.g.
	// `gotest bench ./pkg/parser -run Parse` filters by suite while -bench
	// filters by benchmark name.
	userBenchFilter := ExtractBenchFilter(runFlags)
	runFlags = StripBenchFilter(runFlags)
	targets := benchTargets(prep.compiled, plan.benchRun(sel), runFlags, userBenchFilter)
	withExec(targets, pf.Exec)

	noSuite := slices.Concat(overlay.NoSuitePackages, cfg.NoTestFiles)
	order := flushOrder(cfg, overlay, noSuite)
	collector.SetFlushOrder(order)
	collector.EmitSkippedSuites(plan.SkippedSuites())
	bookBuildFailures(collector, overlay.BrokenPackages, prep.compileFailures)
	bookNoSuites(collector, overlay, noSuite, order)
	nothingToRun := bookUnbenched(collector, overlay.SuitePackages, benchBuild, prep.compiled, targets)

	// "Nothing to run" is a clean outcome only when every matched package
	// became runnable and none of them reports a verdict of its own. A
	// booked build failure makes the run a failure regardless of target
	// count.
	if len(targets) == 0 && !collector.AnyFailed() && len(noSuite) == 0 && !nothingToRun {
		if cfg.OutputMode != RunCaptureJSON && setupErr == nil {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		result := finish(PipelineResult{}, nil)
		if result.ExitCode == 0 {
			result.CapturedJSON = nil
		}
		return result, nil
	}

	setupCoverage(targets, overlay, pf.UserCoverProfile)
	if pf.UserCoverProfile != "" {
		defer mergeCoverProfiles(targets, pf.UserCoverProfile)
	}

	// Serial dispatch, one window per slot: StartKeys(needed ∖ alive)
	// before each bench suite, TeardownKeys(alive ∖ needed-by-any-later-
	// target) after it. A fixture is resident exactly from the first slot
	// that needs it through the last. A setup that timed out is still busy
	// with the up-front phase and would answer no command: its windows are
	// left closed, and every target that needed one is given up.
	SortTargetsSerial(targets)
	slots := make([]*PlannedSuite, len(targets))
	for i := range targets {
		slots[i] = plan.Suite(targets[i].Package, targets[i].SuiteName)
	}
	needs, laterNeeds := benchSlotPlan(slots, win.Fixtures)
	alive := make(map[string]bool, len(win.Bulk))
	for k := range win.Bulk {
		alive[k] = true
	}
	// A window that fails to open is a setup failure, one that fails to
	// close a teardown failure: each carries the fixtures' reports of its
	// own phase.
	var openErrs, closeErrs []error
	beforeSlot := func(i int) {
		if setupProc == nil || prep.setupTimedOut || ctx.Err() != nil {
			return
		}
		start := diffKeys(needs[i], alive)
		if len(start) == 0 {
			return
		}
		// Marked alive on failure too: the subprocess counts a failed
		// fixture as started, so retrying on a later slot would only
		// re-report the same failure.
		for _, k := range start {
			alive[k] = true
		}
		if err := setupProc.StartKeys(ctx, start, resolvedSetupTimeout); err != nil {
			openErrs = append(openErrs, fmt.Errorf("bench fixture window open (%s): %w", targets[i].SuiteName, err))
		}
	}
	afterSlot := func(i int) {
		if setupProc == nil || prep.setupTimedOut || ctx.Err() != nil {
			return
		}
		release := diffKeys(alive, laterNeeds[i+1])
		if len(release) == 0 {
			return
		}
		for _, k := range release {
			delete(alive, k)
		}
		if err := setupProc.TeardownKeys(ctx, release, setupProc.teardownBudget()); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("bench fixture window close (%s): %w", targets[i].SuiteName, err))
		}
	}
	// A target reads its fixtures through a state file of its own, holding
	// exactly the keys it declares; one whose fixture never came up is given
	// up, naming that fixture, while the fixtures that did come up serve the
	// targets that read only them.
	fixtures := settledFixtureSession(setupProc, setupErr, prep.setupTimedOut)
	envFor := func(i int) ([]string, string) {
		t := &targets[i]
		var requiredKeys []string
		if slots[i] != nil {
			requiredKeys = slots[i].SharedKeys
		}
		if missing := fixtures.missing(requiredKeys); missing != "" {
			return nil, neverRan(t.SuiteName, missing)
		}
		env, err := fixtures.env(baseEnv, t.Package, t.SuiteName, requiredKeys)
		if err != nil {
			return nil, fmt.Sprintf("%s never ran: shared fixture state: %s", t.SuiteName, err)
		}
		return env, ""
	}
	RunBenchSuites(ctx, targets, collector, beforeSlot, afterSlot, envFor)
	setupErr = errors.Join(append([]error{setupErr}, openErrs...)...)
	dispatchErr := ctx.Err()
	collector.Finalize()

	exitCode := collector.takeCensus(cfg, overlay.Declared, exitCodeAfterDispatch(collector.WorstExitCode(), dispatchErr), dispatchErr)
	running := collector.bookDeadline(dispatchErr)
	result := PipelineResult{ExitCode: exitCode}
	applyDeadlineFailure(collector, &result, cfg, dispatchErr, running, "")
	applyInterrupt(collector, &result, dispatchErr)
	return finish(result, errors.Join(closeErrs...)), nil
}

// bookUnbenched books each suite package that runs no benchmark as passing,
// as under go test -bench: one left unbuilt, or built with no target. It
// reports whether there was any.
func bookUnbenched(c *OutputCollector, suitePackages, built []string, compiled []CompileResult, targets []SuiteTarget) bool {
	benched := map[string]bool{}
	for i := range targets {
		benched[targets[i].Package] = true
	}
	found := false
	for i := range compiled {
		if pkg := compiled[i].Package; !benched[pkg] {
			c.RecordNothingToRun(pkg)
			found = true
		}
	}
	for _, pkg := range suitePackages {
		if !slices.Contains(built, pkg) {
			c.RecordNothingToRun(pkg)
			found = true
		}
	}
	return found
}

// neverRan is what a suite reports that was given up before it started,
// naming the fixture by its type: key is its state key, import path and all.
func neverRan(suite, key string) string {
	return suite + " never ran: shared fixture " + key[strings.LastIndex(key, ".")+1:] + " did not come up"
}

// missingFixture returns the first of keys whose fixture is not up, "" when
// all are.
func missingFixture(proc *SharedFixtureProcess, keys []string) string {
	for _, key := range keys {
		if proc == nil || !proc.Came(key) {
			return key
		}
	}
	return ""
}

// exitCodeAfterDispatch folds the context error seen when the last verdict
// landed into the suites' worst exit code: an interrupt is 130 whatever the
// suites it cut short reported. A deadline keeps the verdict for
// applyDeadlineFailure to fail.
func exitCodeAfterDispatch(worst int, dispatchErr error) int {
	if errors.Is(dispatchErr, context.Canceled) {
		return 130
	}
	return worst
}

// runStreaming overlaps the compile with the run: each package's suites
// start as its binary lands, then the suites held back run scope by scope.
func runStreaming(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (PipelineResult, error) { //nolint:gocritic // hugeParam: stable API
	r := newStreamRun(ctx, cfg, overlay, pf)
	defer r.cancel()
	defer r.releaseCancel()
	link := stageFlags(r.pf.BuildFlags, r.pf.RunFlags, overlay.WorkDir)
	compiled, span := spanOf(CompilePackagesStream(r.ctx, r.bookUnbuilt(), overlay.OverlayFlag, link, overlay.WorkDir, binaryCacheDir(overlay.BinaryCacheRoot, link), cfg.CompileParallel), time.Now)
	held := r.intake(compiled)
	r.runScopes(held)
	r.releaseHolds()
	result := r.finish()
	result.CompileSpan = span()
	return result, nil
}

// deferredTarget is a suite held back past the stream, with its slot in
// its package's results.
type deferredTarget struct {
	t   SuiteTarget
	idx int
}

// sortDeferred orders ds by (Package, SuiteName), the order a scope's
// held-back suites are dispatched in.
func sortDeferred(ds []deferredTarget) []deferredTarget {
	sort.Slice(ds, func(a, b int) bool {
		if ds[a].t.Package != ds[b].t.Package {
			return ds[a].t.Package < ds[b].t.Package
		}
		return ds[a].t.SuiteName < ds[b].t.SuiteName
	})
	return ds
}

// withExec gives every target the command its binary runs under.
func withExec(targets []SuiteTarget, exec []string) {
	for i := range targets {
		targets[i].Exec = exec
	}
}

// interruptedPkg is the synthetic package a stopped run is booked under.
const interruptedPkg = "interrupted"

// applyInterrupt fails a run an interrupt stopped while suites ran, naming
// them. A stopped suite ends with its tests' own status, so its stream may
// end green; this keeps a -json consumer from reading the run as a pass,
// and leaves every verdict the tests reported as it is.
func applyInterrupt(c *OutputCollector, result *PipelineResult, dispatchErr error) {
	if !errors.Is(dispatchErr, context.Canceled) {
		return
	}
	cut := c.cutShortUnits()
	if len(cut) == 0 {
		return
	}
	failRun(c, result, interruptedPkg, "run interrupted while running: "+unitNames(cut))
}
