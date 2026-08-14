package gotestrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	Streaming       bool
	OutputMode      RunMode
	FuzzFuncsByPkg  map[string]map[string][]string
}

type PipelineResult struct {
	ExitCode     int
	CapturedJSON []byte
}

// applyTeardownFailure surfaces a shared fixture teardown failure and makes a
// run that would otherwise have passed fail instead. Resources the fixtures
// hold outlive the test process, so leaving them behind must not report ok.
func applyTeardownFailure(result *PipelineResult, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
	// The captured stream is the single source every renderer derives from —
	// spec, summary, markdown artifacts, saved --input replays. A failure that
	// only mutated the exit code left all of them saying "all passed" beside
	// exit 1, so it goes into the stream itself as a failed synthetic package.
	if result.CapturedJSON != nil {
		result.CapturedJSON = appendRunFailureEvents(result.CapturedJSON, "shared fixtures", err.Error())
	}
}

// fixtureBarrier performs the bulk→tail window transition on the setup
// process: release what only the bulk needed (the subprocess applies
// reverse-DAG order), then start what only the tail needs (DAG order).
// Skipped entirely on cancellation — the terminal Teardown owns shutdown.
// refreshStateFile is batch mode's concern: its suites read the one global
// state file, which must gain the late-started fixtures' state.
func fixtureBarrier(ctx context.Context, proc *SharedFixtureProcess, bulkAlive, tailAlive map[string]bool, setupTimeout time.Duration, refreshStateFile bool) error {
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
		err := proc.StartKeys(acquire, setupTimeout)
		if err == nil && refreshStateFile {
			err = proc.RefreshStateFile()
		}
		if err != nil {
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

// appendRunFailureEvents appends test2json-shaped events recording a run-level
// failure that happened outside any test binary, after its stream ended.
func appendRunFailureEvents(stream []byte, pkg, msg string) []byte {
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
	stream = append(stream, ev("fail", "")...)
	return stream
}

func RunPipeline(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult) (PipelineResult, error) {
	if !cfg.CI && os.Getenv(protocol.EnvCI) == "" {
		if v := os.Getenv("CI"); v != "" && v != "0" && v != "false" {
			cfg.CI = true
		}
	}
	pf := ParseExecFlags(cfg.GoTestArgs)

	if cfg.Streaming {
		return runStreaming(ctx, cfg, overlay, pf)
	}
	return runBatch(ctx, cfg, overlay, pf)
}

func buildExtraEnv(cfg PipelineConfig, proc *SharedFixtureProcess) map[string]string {
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

func buildBaseEnv(cfg PipelineConfig) []string {
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
// or the packages that did compile. Only a fixture setup failure is fatal —
// without the fixtures no surviving suite can run.
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
		compiled, compileFailures = CompilePackages(ctx, overlay.SuitePackages, overlay.OverlayFlag, buildFlags, overlay.WorkDir, compileParallel)
	}()

	if len(fixtures) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			setupProc, setupErr = StartSharedFixtures(ctx, overlay.WorkDir, fixtures, setupTimeout)
			if setupErr != nil {
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
		if setupProc != nil {
			_ = setupProc.Teardown()
		}
		// Scheduling context: fixture setup deadlines are wall-clock verdicts
		// too, and a starved build looks exactly like a broken one.
		return nil, nil, nil, nil, fmt.Errorf("shared fixture setup: %w %s", setupErr, schedinfo.Summary())
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

func runBatch(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (result PipelineResult, err error) { //nolint:gocritic // hugeParam: stable API
	win := planFixtureWindows(overlay, pf.UserRunFilter)
	win.reportSkipped()
	compiled, compileFailures, setupProc, cancelPrepare, err := prepareTestRun(ctx, overlay, win.Fixtures, pf.BuildFlags, cfg.SetupTimeout, cfg.CompileParallel)
	if err != nil {
		return PipelineResult{ExitCode: 2}, err
	}
	defer cancelPrepare()
	// barrierErr collects window-boundary failures (early teardown, tail
	// start); they merge with the terminal Teardown's verdict below.
	var barrierErr error
	if setupProc != nil {
		defer func() { applyTeardownFailure(&result, errors.Join(barrierErr, setupProc.Teardown())) }()
	}

	select {
	case <-ctx.Done():
		return PipelineResult{ExitCode: 130}, nil
	default:
	}

	extraEnv := buildExtraEnv(cfg, setupProc)

	totalSuites := 0
	for _, suites := range overlay.SuitesByPkg {
		totalSuites += len(suites)
	}
	runFlags := pf.RunFlags
	if cfg.OutputMode == RunCaptureJSON {
		runFlags = append(append([]string(nil), runFlags...), "-v")
	}
	maxParallel := computeDispatchConcurrency(&runFlags, cfg.Parallel, totalSuites, SanitizerActive(pf.BuildFlags))
	targets := BuildSuiteTargets(compiled, overlay.SuitesByPkg, overlay.DirsByPkg, cfg.FuzzFuncsByPkg, overlay.ExclusiveSuitesByPkg, runFlags, pf.UserRunFilter)

	collector := NewOutputCollector(cfg.OutputMode, pf.Verbose)
	collector.StdlibTestsByPkg = overlay.StdlibTestsByPkg
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

	// The barrier re-windows shared fixtures between the parallel bulk and
	// the exclusive tail. Alive(tail) comes from the actual exclusive targets
	// — the plan narrowed by compile results — so a fixture whose only tail
	// suite never became runnable is released, not started.
	var tailTargets []SuiteTarget
	for i := range targets {
		if targets[i].Exclusive {
			tailTargets = append(tailTargets, targets[i])
		}
	}
	barrier := func() {
		if len(tailTargets) == 0 {
			return // nothing dispatches after the bulk; run-end teardown owns the rest
		}
		tailAlive := aliveFromTargets(tailTargets, overlay.SuiteRequiredSharedFixtureKeys, win.Fixtures)
		barrierErr = fixtureBarrier(ctx, setupProc, win.Bulk, tailAlive, resolveSetupTimeout(cfg.SetupTimeout), true)
	}

	RunSuites(ctx, targets, extraEnv, maxParallel, collector, barrier)
	collector.Finalize(overlay.NoSuitePackages)

	return PipelineResult{
		ExitCode:     collector.WorstExitCode(),
		CapturedJSON: collector.CapturedJSON(),
	}, nil
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

	fixtureStarted := make(chan struct{})
	var setupProc *SharedFixtureProcess
	var fixtureStartErr error
	var fixtureWg sync.WaitGroup
	var sharedSetupFailed atomic.Bool

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	if len(win.Fixtures) > 0 {
		fixtureWg.Add(1)
		go func() {
			defer fixtureWg.Done()
			var err error
			// The subprocess is bound to the pipeline ctx, not streamCtx: a
			// setup failure cancels streamCtx to stop suite scheduling, and that
			// must not double as a shutdown signal — Teardown below is the one
			// owner of shutdown, and it runs only after every suite has stopped.
			// The pipeline ctx stays attached as the safety net so an abnormal
			// runner death still releases the process group.
			setupProc, err = StartSharedFixtures(ctx, overlay.WorkDir, win.Fixtures, resolvedSetupTimeout)
			if err != nil {
				fixtureStartErr = err
				sharedSetupFailed.Store(true)
				streamCancel()
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
					fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup failed: %v\n", err)
					sharedSetupFailed.Store(true)
					streamCancel()
				}
			case <-streamCtx.Done():
			case <-setupDeadline:
				fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup timed out after %v\n", resolvedSetupTimeout)
				sharedSetupFailed.Store(true)
				streamCancel()
			}
		}()
	} else {
		close(fixtureStarted)
	}

	compileCh := CompilePackagesStream(streamCtx, overlay.SuitePackages, overlay.OverlayFlag, pf.BuildFlags, overlay.WorkDir, cfg.CompileParallel)

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
			// compile is a failed package, not a skipped one, and batch mode
			// books the same verdict on the same input — the two modes must
			// agree.
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
				defer func() {
					if !recorded {
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
						return
					}

					for _, key := range requiredKeys {
						ch := setupProc.Ready(key)
						if ch == nil {
							return
						}
						select {
						case <-ch:
						case <-setupProc.AllDone():
							// Setup finished. Re-check without blocking: a
							// select picks at random among ready cases, and by
							// the time the sentinel arrives every fixture that
							// did come up has already had its channel closed.
							select {
							case <-ch:
							default:
								// This one never came up — the process
								// crashed, most likely. Waiting on it would
								// hang the run until the outer deadline.
								return
							}
						case <-streamCtx.Done():
							return
						}
					}

					stateFile, err := setupProc.WriteStateFileForKeys(t.SuiteName, requiredKeys)
					if err != nil {
						fmt.Fprintf(os.Stderr, "WARN: write state file for %s: %s\n", t.SuiteName, err)
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
	var barrierErr error
	if len(deferredExclusive) > 0 && setupProc != nil && !sharedSetupFailed.Load() {
		tailTargets := make([]SuiteTarget, 0, len(deferredExclusive))
		for i := range deferredExclusive {
			tailTargets = append(tailTargets, deferredExclusive[i].t)
		}
		tailAlive := aliveFromTargets(tailTargets, overlay.SuiteRequiredSharedFixtureKeys, win.Fixtures)
		barrierErr = fixtureBarrier(streamCtx, setupProc, win.Bulk, tailAlive, resolvedSetupTimeout, false)
	}

	// Exclusive suites own the machine: after the stream has fully drained,
	// one at a time, in deterministic order. Shared fixture processes stay up
	// — they are infrastructure the suites talk to, not competing suites —
	// and tear down only after the last exclusive finishes.
	sort.Slice(deferredExclusive, func(a, b int) bool {
		ta, tb := deferredExclusive[a].t, deferredExclusive[b].t
		if ta.Package != tb.Package {
			return ta.Package < tb.Package
		}
		return ta.SuiteName < tb.SuiteName
	}) //nolint:gocritic // mirror of sortTargetIndices over a local pair type
	for i := range deferredExclusive {
		d := &deferredExclusive[i]
		if streamCtx.Err() != nil {
			collector.RecordResult(d.t.Package, d.idx, SuiteResult{ExitCode: 1})
			continue
		}
		env := baseEnv
		if requiredKeys := overlay.SuiteRequiredSharedFixtureKeys[d.t.Package][d.t.SuiteName]; len(requiredKeys) > 0 && setupProc != nil {
			stateFile, err := setupProc.WriteStateFileForKeys(d.t.SuiteName, requiredKeys)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARN: write state file for %s: %s\n", d.t.SuiteName, err)
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

	fixtureWg.Wait()

	// Teardown owns the shared fixture process's shutdown: it signals, then
	// waits out the configured teardown budget. Cancelling streamCtx first
	// would signal the process behind Teardown's back, leaving two owners for
	// one shutdown — and Teardown could no longer tell a process that died on
	// its own from one that simply obeyed the signal it never sent.
	var teardownErr error
	if setupProc != nil {
		teardownErr = errors.Join(barrierErr, setupProc.Teardown())
	}
	streamCancel()

	if pf.UserCoverProfile != "" {
		mergeCoverProfiles(allTargets, pf.UserCoverProfile)
	}

	// "Nothing to run" is a clean outcome only when every matched package
	// became runnable and none of them reports through Finalize. A booked
	// build failure makes the run a failure regardless of target count.
	if !anyTargets && len(overlay.NoSuitePackages) == 0 && !buildFailed {
		if cfg.OutputMode == RunBatchText {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		result := PipelineResult{}
		if sharedSetupFailed.Load() {
			result.ExitCode = 1
		}
		applyTeardownFailure(&result, teardownErr)
		return result, nil
	}

	collector.Finalize(overlay.NoSuitePackages)

	exitCode := collector.WorstExitCode()
	if ctx.Err() != nil && exitCode == 0 {
		exitCode = 130
	}

	if sharedSetupFailed.Load() && exitCode == 0 {
		exitCode = 1
	}
	result := PipelineResult{
		ExitCode:     exitCode,
		CapturedJSON: collector.CapturedJSON(),
	}
	applyTeardownFailure(&result, teardownErr)
	return result, nil
}
