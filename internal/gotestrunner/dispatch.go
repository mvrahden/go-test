package gotestrunner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// dispatchEvent is one step of a run's dispatch.
type dispatchEvent struct {
	Kind      string   // intake, up-front, start, suite-start, suite-end, given-up, stopped, free, teardown
	Package   string   // intake and suite events
	Suite     string   // test function name, suite events
	Group     string   // suite events
	Exclusive bool     // suite events
	Keys      []string // sorted: the fixtures started or freed, or the ones a suite reads
	For       string   // start: what the fixtures start for; intake: "failed" for a build failure
}

// observeDispatch receives every step of a run's dispatch; nil outside tests.
var observeDispatch func(dispatchEvent)

// traceDispatch hands e to the observer when one is set.
func traceDispatch(e dispatchEvent) { //nolint:gocritic // hugeParam: tests only
	if observeDispatch != nil {
		observeDispatch(e)
	}
}

// traceSuite traces a suite event of t, which reads keys.
func traceSuite(kind string, t *SuiteTarget, keys []string) {
	if observeDispatch == nil {
		return
	}
	traceDispatch(dispatchEvent{Kind: kind, Package: t.Package, Suite: t.SuiteName, Group: t.Group, Exclusive: t.Exclusive, Keys: sortedCopy(keys)})
}

func sortedCopy(keys []string) []string {
	out := slices.Clone(keys)
	slices.Sort(out)
	return out
}

// streamRun is one streaming run: what its phases share.
type streamRun struct {
	ctx       context.Context // the stream's: ends at a stop and when the run is over
	cancel    context.CancelFunc
	cfg       PipelineConfig
	pf        ParsedFlags
	overlay   *OverlayResult
	plan      *RunPlan
	sel       selection
	scopes    []suiteScope
	collector *OutputCollector
	baseEnv   []string
	slots     *slots
	fixtures  *fixtureSession
	ledger    *fixtureLedger
	releaser  *fixtureReleaser
	clock     groupClock

	cfgCtx        context.Context // the pipeline's: its stop and deadline decide the run's verdict
	releaseCancel context.CancelFunc
	win           fixtureWindows
	selected      map[string][]*PlannedSuite // per package, the suites the run dispatches
	streamedGroup string                     // the group whose bulk streams with the compile
	noSuite       []string
	coverDir      string
	// countsDir holds the files a text run's processes write their leaves'
	// assertion counts to; "" for a run that reads them from its stream.
	countsDir    string
	coverTargets []SuiteTarget
	// anyTargets: a package built and dispatched a suite; nothingToRun: a
	// package built but none of its suites run.
	anyTargets, nothingToRun, buildFailed bool
	wg                                    sync.WaitGroup
	dispatchErr                           error
	verdictsIn                            bool
}

// suiteMode is how a suite gets to run.
type suiteMode int

const (
	streamed suiteMode = iota // beside the compile: waits for its fixtures, then takes a slot
	deferred                  // a later group's bulk: its fixtures were started for it, takes a slot
	alone                     // an exclusive suite: no slot, nothing else runs
)

// runSuite runs one suite and books exactly one verdict for it: its
// result, stopped, or given up for a fixture that did not come up.
// held is the slot the suite queued for before it was handed here, nil for
// none: it then queues once its fixtures are up.
func (r *streamRun) runSuite(t SuiteTarget, idx int, mode suiteMode, held chan struct{}) { //nolint:gocritic // hugeParam: a copy per suite is nothing beside the process it starts
	if held != nil {
		defer r.slots.leave(held)
	}
	keys := plannedKeys(r.plan, &t)
	stopped := func() {
		traceSuite("stopped", &t, keys)
		r.collector.RecordStopped(t.Package, idx, t.SuiteName)
	}
	if r.ctx.Err() != nil {
		stopped()
		return
	}
	var missing string
	if mode == streamed {
		var ok bool
		if missing, ok = r.fixtures.await(r.ctx, keys); !ok {
			stopped()
			return
		}
	} else {
		missing = r.fixtures.missing(keys)
	}
	if missing != "" {
		traceSuite("given-up", &t, keys)
		r.collector.RecordGivenUp(t.Package, idx, t.SuiteName, neverRan(t.SuiteName, missing))
		return
	}
	env, err := r.fixtures.env(r.baseEnv, t.Package, t.SuiteName, keys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s %s: shared fixture state: %s\n", t.Package, t.SuiteName, err)
		r.collector.RecordResult(t.Package, idx, SuiteResult{ExitCode: 1})
		return
	}
	if mode != alone {
		if held == nil {
			held = r.slots.queue()
			defer r.slots.leave(held)
		}
		select {
		case <-held:
		case <-r.ctx.Done():
			stopped()
			return
		}
	}
	r.clock.run(t.Group, func() {
		traceSuite("suite-start", &t, keys)
		runRecorded(r.ctx, r.collector, t.Package, idx, t, env, r.collector.UsesTest2JSON())
		traceSuite("suite-end", &t, keys)
	})
}

// done tells the ledger the suite's process is gone and queues what that frees.
func (r *streamRun) done(t *SuiteTarget) {
	r.free(r.ledger.Done(t.Package, t.SuiteName))
}

// free queues keys no planned suite left needs for release.
func (r *streamRun) free(keys []string) {
	if len(keys) > 0 {
		traceDispatch(dispatchEvent{Kind: "free", Keys: sortedCopy(keys)})
	}
	r.releaser.Release(keys)
}

// newStreamRun plans a streaming run and books what is known before
// anything is built: the packages without suites and the broken ones.
func newStreamRun(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) *streamRun { //nolint:gocritic // hugeParam: once per run
	r := &streamRun{cfgCtx: ctx, cfg: cfg, pf: pf, overlay: overlay}
	if pf.UserCoverProfile != "" {
		r.coverDir = filepath.Join(overlay.WorkDir, "cover")
		_ = os.MkdirAll(r.coverDir, 0o755)
	}
	if cfg.OutputMode == RunBatchText && os.MkdirAll(filepath.Join(overlay.WorkDir, "counts"), 0o755) == nil {
		r.countsDir = filepath.Join(overlay.WorkDir, "counts")
	}

	setupTimeout := resolveSetupTimeout(cfg.SetupTimeout)
	r.baseEnv = buildBaseEnv(cfg)

	r.plan = overlay.plan()
	sel := selection{Run: pf.UserRunFilter, Skip: pf.UserSkipFilter, Groups: overlay.groups, Only: cfg.Only}
	r.sel = sel
	r.win = planFixtureWindows(r.plan, overlay.SharedFixtures, sel)
	r.win.reportSkipped()
	r.scopes = planSuiteScopes(r.plan, sel)
	// The targets of each package are built from the same selection the
	// scopes and the ledger are.
	r.selected = byPackage(r.plan.testRun(sel))
	// A shared fixture is released once no planned suite left needs it.
	var planned []*PlannedSuite
	for i := range r.scopes {
		planned = append(planned, r.scopes[i].suites()...)
	}
	r.ledger = newFixtureLedger(planned, r.win.Fixtures)

	// The first scope's bulk streams with the compile. Every other suite is
	// collected during the stream and runs after it, scope by scope: a
	// group's bulk, then its exclusive suites one at a time.
	if len(r.scopes) > 0 {
		r.streamedGroup = r.scopes[0].Group
	}

	r.ctx, r.cancel = context.WithCancel(ctx)
	r.fixtures = startFixtureSession(ctx, r.ctx, r.cancel, overlay.WorkDir, r.win, setupTimeout, pf.BuildFlags)

	// Releases go to a process whose up-front phase ended.
	releaseCtx, releaseCancel := context.WithCancel(r.ctx)
	r.releaseCancel = releaseCancel
	r.releaser = startFixtureReleaser(releaseCtx, func() *SharedFixtureProcess { return r.fixtures.releasable(releaseCtx) })

	totalSuites := 0
	for _, s := range r.plan.All() {
		if s.Tests {
			totalSuites++
		}
	}
	maxParallel := computeDispatchConcurrency(&r.pf.RunFlags, cfg.Parallel, totalSuites, pf.ProcessCap, SanitizerActive(pf.BuildFlags), runtime.GOMAXPROCS(0))
	r.slots = newSlots(maxParallel)
	r.buildFailed = len(overlay.BrokenPackages) > 0

	// A package that will never run holds no fixture.
	for i := range overlay.BrokenPackages {
		r.free(r.ledger.DoneExcept(overlay.BrokenPackages[i].PkgPath, nil))
	}

	c := NewOutputCollector(cfg.OutputMode, pf.Verbose, WithStdout(cfg.Stdout), WithEvents(cfg.Events), WithInputs(cfg.Inputs), WithManifest(cfg.Manifest))
	r.collector = c
	c.StdlibTestsByPkg = r.plan.StdlibTests()
	c.holdToTests = cfg.OutputMode == RunBatchText && !mayRunNothing(cfg.GoTestArgs, r.pf.RunFlags) && cfg.Only == nil
	c.censusAside = censusAside(cfg, r.pf.RunFlags)
	// The flush order holds every package the collector will report on.
	r.noSuite = slices.Concat(overlay.NoSuitePackages, cfg.NoTestFiles)
	order := flushOrder(cfg, overlay, r.noSuite)
	c.SetFlushOrder(order)
	c.EmitSkippedSuites(r.plan.SkippedSuites())
	bookBuildFailures(c, overlay.BrokenPackages, nil)
	bookNoSuites(c, overlay, r.noSuite, order)
	return r
}

// scopeQueue is what the stream held back for one group.
type scopeQueue struct{ bulk, tail []deferredTarget }

// bookUnbuilt books each suite package the run dispatches no suite of as
// having nothing to run, and returns the packages to build.
func (r *streamRun) bookUnbuilt() []string {
	dispatched := dispatchedPackages(r.plan.testRun(r.sel))
	for _, pkg := range r.overlay.SuitePackages {
		if !dispatched[pkg] {
			r.collector.RecordNothingToRun(pkg)
			r.nothingToRun = true
			r.free(r.ledger.DoneExcept(pkg, nil))
		}
	}
	return buildSet(r.overlay.SuitePackages, dispatched)
}

// intake reads the compile stream: it books build failures, builds each
// package's targets, starts the first scope's bulk and queues every other
// suite. It returns once the stream ended and the suites it started are done.
func (r *streamRun) intake(compiled <-chan CompileOutcome) map[string]*scopeQueue {
	held := map[string]*scopeQueue{}
	for {
		select {
		case outcome, ok := <-compiled:
			if !ok {
				r.wg.Wait()
				return held
			}
			r.take(outcome, held)
		case <-r.ctx.Done():
			// The stage still ends: what it reports now is dropped.
			go func() {
				for range compiled {
				}
			}()
			r.wg.Wait()
			return held
		}
	}
}

// take handles one compile outcome.
func (r *streamRun) take(outcome CompileOutcome, held map[string]*scopeQueue) { //nolint:gocritic // hugeParam: once per package
	if outcome.Err != nil {
		// Discovery-to-result is a total function: a package that fails to
		// compile is a failed package, not a skipped one, and a bench
		// run books the same verdict on the same input.
		if r.ctx.Err() != nil {
			return // cancellation noise, not a compile verdict
		}
		traceDispatch(dispatchEvent{Kind: "intake", Package: outcome.Package, For: "failed"})
		r.buildFailed = true
		bookBuildFailures(r.collector, nil, []BuildFailure{{Package: outcome.Package, Err: outcome.Err}})
		r.free(r.ledger.DoneExcept(outcome.Package, nil))
		return
	}
	cr := outcome.Result
	traceDispatch(dispatchEvent{Kind: "intake", Package: cr.Package})

	filter, _ := r.sel.runFilter(cr.Package)
	targets := suiteTargets(cr, r.selected[cr.Package], r.pf.RunFlags, filter)
	withExec(targets, r.pf.Exec)

	if len(targets) == 0 {
		r.collector.RecordNothingToRun(cr.Package)
		r.nothingToRun = true
		return
	}
	r.anyTargets = true

	if r.pf.UserCoverProfile != "" {
		baseIdx := len(r.coverTargets)
		for j := range targets {
			targets[j].CoverProfile = filepath.Join(r.coverDir, fmt.Sprintf("%d.out", baseIdx+j))
		}
		r.coverTargets = append(r.coverTargets, targets...)
	}
	for j := range targets {
		targets[j].CountsDir = r.countsDir
	}

	r.collector.Register(cr.Package, len(targets))
	r.clock.count(targets)

	for i := range targets { //nolint:gocritic // hugeParam: stable API
		target := targets[i]
		if target.Exclusive || target.Group != r.streamedGroup {
			// Deferred past the stream: exclusive suites run strictly
			// alone, after every concurrent suite of their group has
			// drained, and a later group after the one before it. Their
			// slot in the collector's per-package count is already
			// registered above; the index travels with them.
			q := held[target.Group]
			if q == nil {
				q = &scopeQueue{}
				held[target.Group] = q
			}
			d := deferredTarget{t: target, idx: i}
			if target.Exclusive {
				q.tail = append(q.tail, d)
			} else {
				q.bulk = append(q.bulk, d)
			}
			continue
		}
		// A suite with no shared fixture to wait for queues now, in the
		// order the compile reported its package.
		var held chan struct{}
		if len(plannedKeys(r.plan, &target)) == 0 {
			held = r.slots.queue()
		}
		r.wg.Add(1)
		go func(t SuiteTarget, idx int) {
			defer r.wg.Done()
			// Last of all, once the suite's process is gone.
			defer r.done(&t)
			r.runSuite(t, idx, streamed, held)
		}(target, i)
	}
}

// allIn records, once, what the pipeline's context says when the last
// verdict is in: a stop during the releases that follow keeps the verdicts
// (see exitCodeAfterDispatch).
func (r *streamRun) allIn() {
	if !r.verdictsIn {
		r.dispatchErr, r.verdictsIn = r.cfgCtx.Err(), true
	}
}

// startFor starts the fixtures the held-back suites ds read that are not up
// yet; what names them in a failure.
func (r *streamRun) startFor(ds []deferredTarget, what string) {
	suites := make([]*PlannedSuite, 0, len(ds))
	for i := range ds {
		suites = append(suites, r.plan.Suite(ds[i].t.Package, strings.TrimPrefix(ds[i].t.SuiteName, "Test")))
	}
	r.fixtures.startFor(r.ctx, suites, r.win.Fixtures, what)
}

// runScopes runs what the stream held back, scope by scope: a later
// group's bulk in parallel, then each group's exclusive suites one at a time.
func (r *streamRun) runScopes(held map[string]*scopeQueue) {
	// The suites past the stream start the fixtures they need that are not
	// up yet just before they run: a later group's bulk at the group's start,
	// an exclusive suite in its slot. The releases go through the ledger.
	r.fixtures.settle()
	left := 0
	for _, q := range held {
		left += len(q.bulk) + len(q.tail)
	}
	if left == 0 {
		r.allIn()
	}

	// Scopes never overlap: a later group starts once the one before it has
	// drained. Exclusive suites own the machine: after their group's bulk,
	// one at a time, in deterministic order. Shared fixture processes stay
	// up — they are infrastructure the suites talk to, not competing suites.
	for n, sc := range r.scopes {
		q := held[sc.Group]
		if q == nil {
			q = &scopeQueue{}
		}
		if n > 0 {
			bulk := sortDeferred(q.bulk)
			r.startFor(bulk, "group "+sc.Group)
			var scopeWg sync.WaitGroup
			for i := range bulk {
				d := &bulk[i]
				scopeWg.Add(1)
				go func(t SuiteTarget, idx int) {
					defer scopeWg.Done()
					// Last of all, once the suite's process is gone.
					defer r.done(&t)
					r.runSuite(t, idx, deferred, nil)
				}(d.t, d.idx)
			}
			scopeWg.Wait()
			if left -= len(bulk); left == 0 && len(bulk) > 0 {
				r.allIn()
			}
		}
		tail := sortDeferred(q.tail)
		for i := range tail {
			d := &tail[i]
			r.startFor(tail[i:i+1], "exclusive tail")
			r.runSuite(d.t, d.idx, alone, nil)
			if left--; left == 0 {
				r.allIn()
			}
			// The slot's process is gone. Its release runs beside the next
			// slot, which needs none of what it frees.
			r.done(&d.t)
		}
	}
	r.allIn()
}

// finish releases the fixtures and turns the collected verdicts into the
// run's result.
func (r *streamRun) finish() PipelineResult {
	// Teardown owns the shared fixture process's shutdown: it signals, then
	// waits out the configured teardown budget. Cancelling the stream first
	// would signal the process behind Teardown's back, leaving two owners for
	// one shutdown — and Teardown could no longer tell a process that died on
	// its own from one that simply obeyed the signal it never sent. Releases
	// still queued or in flight go to the terminal teardown, which releases
	// everything at once: sending them one after another would end the run
	// later.
	r.releaseCancel()
	setupErr, teardownErr := r.fixtures.close(r.releaser.Err())
	r.cancel()

	if r.pf.UserCoverProfile != "" {
		mergeCoverProfiles(r.coverTargets, r.pf.UserCoverProfile)
	}

	// "Nothing to run" is a clean outcome only when every matched package
	// became runnable and none of them reports a verdict of its own. A
	// booked build failure makes the run a failure regardless of target
	// count. A run cut short while compiling takes this branch too; it is
	// not empty, so it does not say so.
	c, dispatchErr := r.collector, r.dispatchErr
	empty := !r.anyTargets && !r.nothingToRun && len(r.noSuite) == 0 && !r.buildFailed
	var result PipelineResult
	if empty {
		if r.cfg.OutputMode == RunBatchText && dispatchErr == nil && !r.fixtures.failedSetup() {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		result = PipelineResult{ExitCode: exitCodeAfterDispatch(0, dispatchErr), Groups: r.clock.runs()}
		if r.fixtures.failedSetup() && result.ExitCode == 0 {
			result.ExitCode = 1
		}
		applyDeadlineFailure(c, &result, r.cfg, dispatchErr, nil)
	} else {
		c.Finalize()
		exitCode := exitCodeAfterDispatch(c.WorstExitCode(), dispatchErr)
		if r.fixtures.failedSetup() && exitCode == 0 {
			exitCode = 1
		}
		exitCode = c.takeCensus(r.cfg, r.overlay.Declared, exitCode, dispatchErr)
		exitCode = c.judgeVacuous(r.cfg.CI, exitCode)
		running := c.bookDeadline(dispatchErr)
		result = PipelineResult{ExitCode: exitCode, Groups: r.clock.runs()}
		applyDeadlineFailure(c, &result, r.cfg, dispatchErr, running)
		applyInterrupt(c, &result, dispatchErr)
	}
	applySetupFailure(c, &result, setupErr)
	applyTeardownFailure(c, &result, teardownErr)
	if !empty || result.ExitCode != 0 {
		result.CapturedJSON = c.CapturedJSON()
	}
	result.Judged, result.Failed = c.judged()
	result.Packages = c.packageVerdicts()
	return result
}
