package gotestrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/schedinfo"
)

const DefaultSetupTimeout = 2 * time.Minute

func resolveSetupTimeout(d time.Duration) time.Duration {
	switch {
	case d > 0:
		return d
	case d < 0:
		return 0
	default:
		return DefaultSetupTimeout
	}
}

func computeDispatchConcurrency(runFlags *[]string, budget, totalSuites int, sanitized bool) int {
	userParallel := ExtractParallelValue(*runFlags)

	if userParallel > 0 && budget == 0 {
		// The user pinned intra-process parallelism; the process count is
		// still ours to choose — halved under instrumentation like every
		// other default (see SanitizerActive).
		if sanitized {
			return runtime.GOMAXPROCS(0)
		}
		return 2 * runtime.GOMAXPROCS(0)
	}

	procs := runtime.GOMAXPROCS(0)
	if sanitized && budget == 0 {
		// Halve both dimensions: the budget (total concurrent test methods)
		// and the process cap. Halving the budget alone would not reduce the
		// process count — ComputeConcurrency caps inter at procs first — and
		// the OS process running an instrumented binary is the costly unit.
		budget = procs
		procs = max(1, procs/2)
	}
	inter, intra := ComputeConcurrency(budget, totalSuites, procs)
	if userParallel == 0 {
		*runFlags = InjectParallel(*runFlags, intra)
	}
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
	BenchesByPkg    map[string][]string
	FuzzFuncsByPkg  map[string]map[string][]string
	// GlobalTimeout names the deadline in the failure of a run that outlives
	// it; the deadline itself arrives on ctx.
	GlobalTimeout time.Duration
}

type PipelineResult struct {
	ExitCode     int
	CapturedJSON []byte
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
func applyDeadlineFailure(c *OutputCollector, result *PipelineResult, cfg PipelineConfig, dispatchErr error, running []CensusCase) { //nolint:gocritic // hugeParam: stable API
	if !errors.Is(dispatchErr, context.DeadlineExceeded) {
		return
	}
	msg := "run deadline exceeded"
	if cfg.GlobalTimeout > 0 {
		msg = fmt.Sprintf("global --timeout exceeded after %v", cfg.GlobalTimeout)
	}
	if len(running) == 0 {
		failRun(c, result, "global --timeout", msg)
		return
	}
	fmt.Fprintf(os.Stderr, "FAIL: %s while running: %s\n", msg, unitNames(running))
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
	fmt.Fprintf(os.Stderr, "FAIL: %s\n", msg)
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
	c.bookRunFailure(pkg, msg, details...)
}

// fixtureBarrier performs the bulk→tail window transition on the setup
// process: release what only the bulk needed (the subprocess applies
// reverse-DAG order), then start what only the tail needs (DAG order).
// Skipped entirely on cancellation — the terminal Teardown owns shutdown.
func fixtureBarrier(ctx context.Context, proc *SharedFixtureProcess, bulkAlive, tailAlive map[string]bool, setupTimeout time.Duration) error {
	if proc == nil || ctx.Err() != nil {
		return nil
	}
	var errs []error
	if release := diffKeys(bulkAlive, tailAlive); len(release) > 0 {
		if err := proc.TeardownKeys(release, proc.teardownBudget()); err != nil {
			errs = append(errs, fmt.Errorf("early shared fixture teardown: %w", err))
		}
	}
	if acquire := diffKeys(tailAlive, bulkAlive); len(acquire) > 0 {
		if err := proc.StartKeys(acquire, setupTimeout); err != nil {
			errs = append(errs, fmt.Errorf("shared fixture start for exclusive tail: %w", err))
		}
	}
	return errors.Join(errs...)
}

// stageFailurePkg names the synthetic package under which a build failure
// that belongs to no single package (e.g. the compile stage's own setup) is
// booked. The space keeps it out of the import-path namespace.
const stageFailurePkg = "go build"

// brokenPackageMessage renders a load-broken package's diagnostics in the
// `go build` shape: a `# path` header followed by one diagnostic per line.
func brokenPackageMessage(bp *gotestgen.BrokenPackage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", bp.PkgPath)
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
			Action  string `json:"Action"`
			Package string `json:"Package"`
			Output  string `json:"Output,omitempty"`
		}{Action: action, Package: pkg, Output: text}
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
		if v := os.Getenv("CI"); v != "" && v != "0" && v != "false" {
			cfg.CI = true
		}
	}
	pf := ParseExecFlags(cfg.GoTestArgs)

	// Only when seed corpora actually replay in this run: a stale entry is
	// what would fail, and the engine's own error names the wrapper, not the
	// field that moved.
	if len(cfg.FuzzFuncsByPkg) > 0 {
		ReportStaleFuzzCorpora(os.Stderr, overlay)
	}

	// A bench run compiles everything before it dispatches, so no build
	// competes with a running benchmark. Every other run overlaps the two.
	if cfg.Bench {
		return runBench(ctx, cfg, overlay, pf)
	}
	return runStreaming(ctx, cfg, overlay, pf)
}

func buildExtraEnv(cfg PipelineConfig, proc *SharedFixtureProcess) map[string]string { //nolint:gocritic // hugeParam: stable API
	env := make(map[string]string)
	if cfg.UpdateSnapshots {
		env[protocol.EnvUpdateSnapshots] = "1"
	}
	if cfg.CI {
		env[protocol.EnvCI] = "1"
	}
	if proc != nil {
		env[protocol.EnvSharedStateFile] = proc.StateFile()
	}
	return env
}

func buildBaseEnv(cfg PipelineConfig) []string { //nolint:gocritic // hugeParam: stable API
	env := os.Environ()
	if cfg.UpdateSnapshots {
		env = append(env, protocol.EnvUpdateSnapshots+"=1")
	}
	if cfg.CI {
		env = append(env, protocol.EnvCI+"=1")
	}
	return env
}

// prepareTestRun compiles the suite packages and starts the given shared
// fixtures concurrently. fixtures is the run's residency plan (see
// planFixtureWindows), not the full overlay set: a fixture no scheduled suite
// requires never starts. Per-package compile failures are package verdicts,
// not run aborts: they are returned for booking and do not stop the fixtures
// or the packages that did compile. Only a fixture setup failure ends the
// run — without the fixtures no surviving suite can run.
func prepareTestRun(ctx context.Context, overlay *OverlayResult, fixtures []gotestgen.SharedFixtureInfo, buildFlags []string, setupTimeout time.Duration, compileParallel int) ([]CompileResult, []BuildFailure, *SharedFixtureProcess, context.CancelFunc, error) {
	setupTimeout = resolveSetupTimeout(setupTimeout)
	ctx, cancel := context.WithCancel(ctx)

	var compiled []CompileResult
	var compileFailures []BuildFailure
	var setupProc *SharedFixtureProcess
	var setupErr error

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		compiled, compileFailures = CompilePackages(ctx, overlay.SuitePackages, overlay.OverlayFlag, buildFlags, overlay.WorkDir, binaryCacheDir(overlay.BinaryCacheRoot, buildFlags), compileParallel)
	}()

	if len(fixtures) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			setupProc, setupErr = StartSharedFixtures(ctx, overlay.WorkDir, fixtures, setupTimeout)
			if setupErr != nil {
				setupErr = fmt.Errorf("shared fixture setup failed: %w", setupErr)
				cancel()
				return
			}
			if err := setupProc.WaitAllReady(ctx, setupTimeout); err != nil {
				setupErr = err
				cancel()
			}
		}()
	}

	wg.Wait()

	if setupErr != nil {
		cancel()
		var reports []string
		if setupProc != nil {
			_ = setupProc.Teardown()
			reports = setupProc.TakeFailures()
		}
		// Scheduling context: fixture setup deadlines are wall-clock verdicts
		// too, and a starved build looks exactly like a broken one.
		return nil, nil, nil, nil, withReports(fmt.Errorf("%w %s", setupErr, schedinfo.Summary()), reports, false)
	}

	return compiled, compileFailures, setupProc, cancel, nil
}

func assignBudgetFiles(targets []SuiteTarget) {
	for i := range targets {
		targets[i].BudgetFile = protocol.BudgetFilePath(targets[i].BinaryPath)
	}
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

func runBench(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (result PipelineResult, err error) { //nolint:gocritic // hugeParam: stable API
	// Bench runs dispatch bench targets, not test suites, and match -run and
	// -bench against Benchmark<Suite>: their residency plan must come from
	// the same selection, or fixtures would follow the wrong schedule.
	win := planBenchFixtureWindows(overlay, pf.UserRunFilter, ExtractBenchFilter(pf.RunFlags))
	win.reportSkipped()
	collector := NewOutputCollector(cfg.OutputMode, pf.Verbose)
	collector.StdlibTestsByPkg = overlay.StdlibTestsByPkg
	compiled, compileFailures, setupProc, cancelPrepare, err := prepareTestRun(ctx, overlay, win.Fixtures, pf.BuildFlags, cfg.SetupTimeout, cfg.CompileParallel)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			result = PipelineResult{ExitCode: exitCodeAfterDispatch(0, ctxErr)}
			applyDeadlineFailure(collector, &result, cfg, ctxErr, nil)
		} else {
			applySetupFailure(collector, &result, err)
		}
		result.CapturedJSON = collector.CapturedJSON()
		return result, nil
	}
	defer cancelPrepare()
	// barrierErr collects window-boundary failures (early teardown, tail
	// start); they merge with the terminal Teardown's verdict below. The
	// snapshot is refreshed after the booking so the result carries it.
	var barrierErr error
	if setupProc != nil {
		defer func() {
			teardownErr := errors.Join(barrierErr, setupProc.Teardown())
			if teardownErr = withReports(teardownErr, setupProc.TakeFailures(), true); teardownErr != nil {
				applyTeardownFailure(collector, &result, teardownErr)
				result.CapturedJSON = collector.CapturedJSON()
			}
		}()
	}

	if prepareErr := ctx.Err(); prepareErr != nil {
		result = PipelineResult{ExitCode: exitCodeAfterDispatch(0, prepareErr)}
		applyDeadlineFailure(collector, &result, cfg, prepareErr, nil)
		result.CapturedJSON = collector.CapturedJSON()
		return result, nil
	}

	extraEnv := buildExtraEnv(cfg, setupProc)

	runFlags := pf.RunFlags
	if cfg.OutputMode == RunCaptureJSON {
		runFlags = append(append([]string(nil), runFlags...), "-v")
	}

	// A user-supplied -bench value arrives in RunFlags via normal flag
	// classification. It must not reach buildSuiteCmd as a raw -test.bench
	// flag — that would be appended after the generated
	// -test.bench=^Benchmark<Suite>$ and silently win, defeating per-suite
	// scoping. Extract and strip it here, then feed it to BuildBenchTargets
	// as a bench-name filter, matched against Benchmark<Suite>. -run
	// (pf.UserRunFilter) is passed through alongside it as an independent
	// suite filter — both compose with AND semantics, e.g.
	// `gotest bench ./pkg/parser -run Parse` filters by suite while -bench
	// filters by benchmark name.
	userBenchFilter := ExtractBenchFilter(runFlags)
	runFlags = StripBenchFilter(runFlags)
	targets := BuildBenchTargets(compiled, cfg.BenchesByPkg, overlay.DirsByPkg, runFlags, pf.UserRunFilter, userBenchFilter)

	collector.EmitSkippedSuites(overlay.SkippedSuitesByPkg)
	bookBuildFailures(collector, overlay.BrokenPackages, compileFailures)

	// "Nothing to run" is a clean outcome only when every matched package
	// became runnable and none of them reports through Finalize. A booked
	// build failure makes the run a failure regardless of target count.
	if len(targets) == 0 && !collector.AnyFailed() && len(overlay.NoSuitePackages) == 0 {
		if cfg.OutputMode != RunCaptureJSON {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		return PipelineResult{}, nil
	}

	assignBudgetFiles(targets)
	setupCoverage(targets, overlay, pf.UserCoverProfile)
	if pf.UserCoverProfile != "" {
		defer mergeCoverProfiles(targets, pf.UserCoverProfile)
	}

	// Serial dispatch, one window per slot: StartKeys(needed ∖ alive)
	// before each bench suite, TeardownKeys(alive ∖ needed-by-any-later-
	// target) after it. A fixture is resident exactly from the first slot
	// that needs it through the last.
	SortTargetsSerial(targets)
	needs, laterNeeds := benchSlotPlan(targets, overlay.SuiteRequiredSharedFixtureKeys, win.Fixtures)
	alive := make(map[string]bool, len(win.Bulk))
	for k := range win.Bulk {
		alive[k] = true
	}
	var windowErrs []error
	beforeSlot := func(i int) {
		if setupProc == nil || ctx.Err() != nil {
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
		err := setupProc.StartKeys(start, resolveSetupTimeout(cfg.SetupTimeout))
		if err == nil {
			err = setupProc.RefreshStateFile()
		}
		if err != nil {
			windowErrs = append(windowErrs, fmt.Errorf("bench fixture window open (%s): %w", targets[i].SuiteName, err))
		}
	}
	afterSlot := func(i int) {
		if setupProc == nil || ctx.Err() != nil {
			return
		}
		release := diffKeys(alive, laterNeeds[i+1])
		if len(release) == 0 {
			return
		}
		for _, k := range release {
			delete(alive, k)
		}
		if err := setupProc.TeardownKeys(release, setupProc.teardownBudget()); err != nil {
			windowErrs = append(windowErrs, fmt.Errorf("bench fixture window close (%s): %w", targets[i].SuiteName, err))
		}
	}
	RunBenchSuites(ctx, targets, extraEnv, collector, beforeSlot, afterSlot)
	barrierErr = errors.Join(windowErrs...)
	dispatchErr := ctx.Err()
	collector.Finalize(overlay.NoSuitePackages)

	exitCode := collector.takeCensus(cfg, overlay.Declared, exitCodeAfterDispatch(collector.WorstExitCode(), dispatchErr), dispatchErr)
	running := collector.bookDeadline(dispatchErr)
	result = PipelineResult{ExitCode: exitCode}
	applyDeadlineFailure(collector, &result, cfg, dispatchErr, running)
	result.CapturedJSON = collector.CapturedJSON()
	return result, nil
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

func runStreaming(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (PipelineResult, error) { //nolint:gocritic // hugeParam: stable API
	var coverDir string
	if pf.UserCoverProfile != "" {
		coverDir = filepath.Join(overlay.WorkDir, "cover")
		_ = os.MkdirAll(coverDir, 0o755)
	}

	resolvedSetupTimeout := resolveSetupTimeout(cfg.SetupTimeout)
	baseEnv := buildBaseEnv(cfg)

	win := planFixtureWindows(overlay, pf.UserRunFilter)
	win.reportSkipped()

	// Exclusive suites collected during the stream, run serially after it.
	type deferredTarget struct {
		t   SuiteTarget
		idx int
	}
	var deferredExclusive []deferredTarget

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	fixtureStarted := make(chan struct{})
	var setupProc *SharedFixtureProcess
	var fixtureStartErr error
	var fixtureWg sync.WaitGroup
	var sharedSetupFailed atomic.Bool
	// A setup that failed or timed out ends the wait for the fixtures that
	// did not come up, and nothing else: those that did stay up, the suites
	// that read only them run, and so does every suite that reads none.
	// setupErr and setupTimedOut are written by the fixture goroutine and
	// read after fixtureWg has drained.
	setupGaveUp := make(chan struct{})
	var setupErr error
	var setupTimedOut bool
	failSetup := func(err error) {
		setupErr = err
		sharedSetupFailed.Store(true)
		close(setupGaveUp)
	}

	if len(win.Fixtures) > 0 {
		fixtureWg.Add(1)
		go func() {
			defer fixtureWg.Done()
			var err error
			// The subprocess is bound to the pipeline ctx, not streamCtx:
			// Teardown below is the one owner of shutdown, and it runs only
			// after every suite has stopped. The pipeline ctx stays attached
			// as the safety net so an abnormal runner death still releases
			// the process group.
			setupProc, err = StartSharedFixtures(ctx, overlay.WorkDir, win.Fixtures, resolvedSetupTimeout)
			if err != nil {
				fixtureStartErr = err
				if ctx.Err() == nil {
					failSetup(fmt.Errorf("shared fixture setup failed: %w", err))
				} else {
					sharedSetupFailed.Store(true)
					streamCancel()
				}
			}
			close(fixtureStarted)
			if err != nil {
				return
			}
			var setupDeadline <-chan time.Time
			if resolvedSetupTimeout > 0 {
				timer := time.NewTimer(resolvedSetupTimeout)
				defer timer.Stop()
				setupDeadline = timer.C
			}
			select {
			case <-setupProc.AllDone():
				if err := setupProc.SetupErr(); err != nil {
					failSetup(fmt.Errorf("shared fixture setup failed: %w", err))
				}
			case <-streamCtx.Done():
			case <-setupDeadline:
				setupTimedOut = true
				failSetup(fmt.Errorf("shared fixture setup timed out after %v %s", resolvedSetupTimeout, schedinfo.Summary()))
			}
		}()
	} else {
		close(fixtureStarted)
	}

	compileCh := CompilePackagesStream(streamCtx, overlay.SuitePackages, overlay.OverlayFlag, pf.BuildFlags, overlay.WorkDir, binaryCacheDir(overlay.BinaryCacheRoot, pf.BuildFlags), cfg.CompileParallel)

	totalSuites := 0
	for _, suites := range overlay.SuitesByPkg {
		totalSuites += len(suites)
	}
	maxParallel := computeDispatchConcurrency(&pf.RunFlags, cfg.Parallel, totalSuites, SanitizerActive(pf.BuildFlags))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	anyTargets := false
	buildFailed := len(overlay.BrokenPackages) > 0
	var allTargets []SuiteTarget

	collector := NewOutputCollector(cfg.OutputMode, pf.Verbose)
	collector.StdlibTestsByPkg = overlay.StdlibTestsByPkg
	collector.EmitSkippedSuites(overlay.SkippedSuitesByPkg)
	// Broken packages flush ahead of the suite packages: their verdicts are
	// known before any suite runs, and the flush order must contain every
	// package the collector will report on.
	flushOrder := make([]string, 0, len(overlay.BrokenPackages)+len(overlay.SuitePackages))
	for i := range overlay.BrokenPackages {
		flushOrder = append(flushOrder, overlay.BrokenPackages[i].PkgPath)
	}
	flushOrder = append(flushOrder, overlay.SuitePackages...)
	collector.SetFlushOrder(flushOrder)
	bookBuildFailures(collector, overlay.BrokenPackages, nil)

loop:
	for {
		var outcome CompileOutcome
		var ok bool
		select {
		case outcome, ok = <-compileCh:
			if !ok {
				break loop
			}
		case <-streamCtx.Done():
			break loop
		}

		if outcome.Err != nil {
			// Discovery-to-result is a total function: a package that fails to
			// compile is a failed package, not a skipped one, and a bench
			// run books the same verdict on the same input.
			if streamCtx.Err() != nil {
				continue // cancellation noise, not a compile verdict
			}
			buildFailed = true
			bookBuildFailures(collector, nil, []BuildFailure{{Package: outcome.Package, Err: outcome.Err}})
			continue
		}
		cr := outcome.Result

		singleCompiled := []CompileResult{cr}
		singleSuites := map[string][]string{cr.Package: overlay.SuitesByPkg[cr.Package]}
		targets := BuildSuiteTargets(singleCompiled, singleSuites, overlay.DirsByPkg, cfg.FuzzFuncsByPkg, overlay.ExclusiveSuitesByPkg, pf.RunFlags, pf.UserRunFilter)

		if len(targets) == 0 {
			continue
		}
		anyTargets = true

		assignBudgetFiles(targets)

		if pf.UserCoverProfile != "" {
			baseIdx := len(allTargets)
			for j := range targets {
				targets[j].CoverProfile = filepath.Join(coverDir, fmt.Sprintf("%d.out", baseIdx+j))
			}
			allTargets = append(allTargets, targets...)
		}

		collector.Register(cr.Package, len(targets))

		for i := range targets { //nolint:gocritic // hugeParam: stable API
			target := targets[i]
			if target.Exclusive {
				// Deferred past the stream: exclusive suites run strictly
				// alone, after every concurrent suite has drained. Their
				// slot in the collector's per-package count is already
				// registered above; the index travels with them.
				deferredExclusive = append(deferredExclusive, deferredTarget{t: target, idx: i})
				continue
			}
			wg.Add(1)
			go func(t SuiteTarget, idx int) {
				defer wg.Done()
				recorded := false
				// missing names the shared fixture the suite is given up for.
				missing := ""
				defer func() {
					switch {
					case recorded:
					case missing != "":
						collector.RecordGivenUp(t.Package, idx, t.SuiteName, neverRan(t.SuiteName, missing))
					default:
						collector.RecordResult(t.Package, idx, SuiteResult{ExitCode: 1})
					}
				}()

				requiredKeys := overlay.SuiteRequiredSharedFixtureKeys[t.Package][t.SuiteName]
				var env []string
				if len(requiredKeys) > 0 {
					select {
					case <-fixtureStarted:
					case <-streamCtx.Done():
						return
					}
					if fixtureStartErr != nil {
						missing = requiredKeys[0]
						return
					}

					for _, key := range requiredKeys {
						// A fixture the process does not know has no channel,
						// and a nil channel never delivers.
						select {
						case <-setupProc.Ready(key):
						case <-setupProc.AllDone():
						case <-setupGaveUp:
						case <-streamCtx.Done():
							return
						}
						// A select picks at random among ready cases; by the
						// time setup is over, every fixture that came up has
						// had its channel closed.
						if !setupProc.Came(key) {
							missing = key
							return
						}
					}

					stateFile, err := setupProc.WriteSuiteStateFile(t.Package, t.SuiteName, requiredKeys)
					if err != nil {
						fmt.Fprintf(os.Stderr, "FAIL: %s %s: shared fixture state: %s\n", t.Package, t.SuiteName, err)
						return
					}

					env = make([]string, len(baseEnv), len(baseEnv)+1)
					copy(env, baseEnv)
					env = append(env, protocol.EnvSharedStateFile+"="+stateFile)
				} else {
					env = baseEnv
				}

				select {
				case sem <- struct{}{}:
				case <-streamCtx.Done():
					return
				}
				defer func() { <-sem }()

				r := RunSingleSuite(streamCtx, t, env, collector.UsesTest2JSON())
				collector.RecordResult(t.Package, idx, r)
				recorded = true
			}(target, i)
		}
	}

	wg.Wait()

	// Bulk→tail barrier: re-window shared fixtures for the exclusive tail.
	// Alive(tail) comes from the actual deferred targets — suites whose
	// packages never compiled are not in it. Skipped entirely on cancellation
	// or when nothing dispatches after the bulk: the terminal Teardown owns
	// whatever is still resident.
	//
	// A failed setup leaves the process serving commands, so the tail's own
	// fixtures still start; one that timed out is still busy with the
	// up-front phase and would answer no command.
	fixtureWg.Wait()
	tailTargets := make([]SuiteTarget, len(deferredExclusive))
	tailOrder := make([]int, len(deferredExclusive))
	for i := range deferredExclusive {
		tailTargets[i], tailOrder[i] = deferredExclusive[i].t, i
	}
	var barrierErr error
	if len(deferredExclusive) > 0 && setupProc != nil && !setupTimedOut {
		tailAlive := aliveFromTargets(tailTargets, overlay.SuiteRequiredSharedFixtureKeys, win.Fixtures)
		barrierErr = fixtureBarrier(streamCtx, setupProc, win.Bulk, tailAlive, resolvedSetupTimeout)
	}

	// Exclusive suites own the machine: after the stream has fully drained,
	// one at a time, in deterministic order. Shared fixture processes stay up
	// — they are infrastructure the suites talk to, not competing suites —
	// and tear down only after the last exclusive finishes.
	sortTargetIndices(tailTargets, tailOrder)
	for _, i := range tailOrder {
		d := &deferredExclusive[i]
		requiredKeys := overlay.SuiteRequiredSharedFixtureKeys[d.t.Package][d.t.SuiteName]
		if streamCtx.Err() != nil {
			collector.RecordResult(d.t.Package, d.idx, SuiteResult{ExitCode: 1})
			continue
		}
		if missing := missingFixture(setupProc, requiredKeys); missing != "" {
			collector.RecordGivenUp(d.t.Package, d.idx, d.t.SuiteName, neverRan(d.t.SuiteName, missing))
			continue
		}
		env := baseEnv
		if len(requiredKeys) > 0 {
			stateFile, err := setupProc.WriteSuiteStateFile(d.t.Package, d.t.SuiteName, requiredKeys)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %s %s: shared fixture state: %s\n", d.t.Package, d.t.SuiteName, err)
				collector.RecordResult(d.t.Package, d.idx, SuiteResult{ExitCode: 1})
				continue
			}
			env = make([]string, len(baseEnv), len(baseEnv)+1)
			copy(env, baseEnv)
			env = append(env, protocol.EnvSharedStateFile+"="+stateFile)
		}
		r := RunSingleSuite(streamCtx, d.t, env, collector.UsesTest2JSON())
		collector.RecordResult(d.t.Package, d.idx, r)
	}
	// The last verdict is in; what the context says from here on is not the
	// run's business (see exitCodeAfterDispatch).
	dispatchErr := ctx.Err()

	// Teardown owns the shared fixture process's shutdown: it signals, then
	// waits out the configured teardown budget. Cancelling streamCtx first
	// would signal the process behind Teardown's back, leaving two owners for
	// one shutdown — and Teardown could no longer tell a process that died on
	// its own from one that simply obeyed the signal it never sent.
	var teardownErr error
	if setupProc != nil {
		teardownErr = errors.Join(barrierErr, setupProc.Teardown())
		reports := setupProc.TakeFailures()
		setupErr = withReports(setupErr, reports, false)
		teardownErr = withReports(teardownErr, reports, true)
	}
	streamCancel()

	if pf.UserCoverProfile != "" {
		mergeCoverProfiles(allTargets, pf.UserCoverProfile)
	}

	// "Nothing to run" is a clean outcome only when every matched package
	// became runnable and none of them reports through Finalize. A booked
	// build failure makes the run a failure regardless of target count.
	if !anyTargets && len(overlay.NoSuitePackages) == 0 && !buildFailed {
		// A run cut short while compiling reaches here too; it is not empty.
		if cfg.OutputMode == RunBatchText && dispatchErr == nil && !sharedSetupFailed.Load() {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		result := PipelineResult{ExitCode: exitCodeAfterDispatch(0, dispatchErr)}
		if sharedSetupFailed.Load() && result.ExitCode == 0 {
			result.ExitCode = 1
		}
		applyDeadlineFailure(collector, &result, cfg, dispatchErr, nil)
		applySetupFailure(collector, &result, setupErr)
		applyTeardownFailure(collector, &result, teardownErr)
		if result.ExitCode != 0 {
			result.CapturedJSON = collector.CapturedJSON()
		}
		return result, nil
	}

	collector.Finalize(overlay.NoSuitePackages)

	exitCode := exitCodeAfterDispatch(collector.WorstExitCode(), dispatchErr)
	if sharedSetupFailed.Load() && exitCode == 0 {
		exitCode = 1
	}
	exitCode = collector.takeCensus(cfg, overlay.Declared, exitCode, dispatchErr)
	running := collector.bookDeadline(dispatchErr)
	result := PipelineResult{ExitCode: exitCode}
	applyDeadlineFailure(collector, &result, cfg, dispatchErr, running)
	applySetupFailure(collector, &result, setupErr)
	applyTeardownFailure(collector, &result, teardownErr)
	result.CapturedJSON = collector.CapturedJSON()
	return result, nil
}
