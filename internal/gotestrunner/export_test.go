package gotestrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
)

type ExportOverlayJSON = overlayJSON
type ExportFixtureStateEntry = fixtureStateEntry

var ExportCompileConcurrency = compileConcurrency
var ExportBuildSuiteCmd = buildSuiteCmd
var ExportTest2JSONPath = test2jsonPath
var ExportResolveTest2JSON = resolveTest2JSON
var ExportConverterArgv = converterArgv
var ExportConverterCmd = converterCmd
var ExportBookBuildFailures = bookBuildFailures
var ExportBrokenPackageMessage = brokenPackageMessage
var ExportApplyTeardownFailure = applyTeardownFailure
var ExportSplitTopLevelOr = splitTopLevelOr
var ExportProcessRunFilter = processRunFilter
var ExportAssignCoverProfiles = assignCoverProfiles
var ExportResolveSetupTimeout = resolveSetupTimeout
var ExportStampBlock = stampBlock

// ExportSetSharedFixtureDrainDelay sets the shared fixture process's drain
// delay and returns the call that restores it.
func ExportSetSharedFixtureDrainDelay(d time.Duration) (restore func()) {
	prev := sharedFixtureDrainDelay
	sharedFixtureDrainDelay = d
	return func() { sharedFixtureDrainDelay = prev }
}

var ExportBuildBaseEnv = buildBaseEnv
var ExportCacheRoot = cacheRoot
var ExportBinaryCacheKey = binaryCacheKey
var ExportBinaryCacheDir = binaryCacheDir
var ExportLockFile = lockFile
var ExportCopyFile = copyFile

type ExportBinarySum = binarySum

var (
	ExportSumFile     = sumFile
	ExportCopyFileSum = copyFileSum
	ExportReadSum     = readSum
	ExportWriteSum    = writeSum
)

var ExportClearBinaryCache = clearBinaryCache
var ExportFilterPackageLevelEvents = filterPackageLevelEvents
var ExportIsPackageSummaryLine = protocol.IsPackageSummaryLine
var ExportComputeDispatchConcurrency = computeDispatchConcurrency

// ExportKillTree and ExportProcessDone let the teardown tests act on the shared
// fixture subprocess directly: kill it outright, and see when it is reaped.
func ExportKillTree(p *SharedFixtureProcess) error { return p.tree.Kill() }

func ExportProcessDone(p *SharedFixtureProcess) <-chan struct{} { return p.done }

// ExportSetTeardownTimeout overrides the budget Teardown enforces. The
// subprocess reports its own budget on the _done line, so a test that wants to
// drive the force-kill path has to shrink it afterwards rather than sleep out
// the minutes the fixture asked for.
func ExportSetTeardownTimeout(p *SharedFixtureProcess, d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.teardownTimeout = d
}

// ExportTeardownBudget is how long Teardown waits for the process to exit
// before it force-kills it.
func ExportTeardownBudget(p *SharedFixtureProcess) time.Duration { return p.teardownBudget() }

var ExportBuildFuzzArgs = buildFuzzArgs
var ExportDefaultFuzzJobs = defaultFuzzJobs
var ExportResolveFuzzJobs = resolveFuzzJobs
var ExportLineWriterMaxBuf = lineWriterMaxBuf

func ExportNewLineWriter(dst io.Writer, label string, mu *sync.Mutex) io.WriteCloser {
	return newLineWriter(dst, label, mu)
}

func ExportAutoDetectCI(cfg PipelineConfig) PipelineConfig { //nolint:gocritic // hugeParam: stable API
	if !cfg.CI && os.Getenv(protocol.EnvCI) == "" && os.Getenv("CI") != "" {
		cfg.CI = true
	}
	return cfg
}

func ExportNewSharedFixtureProcess(sharedDir string, state map[string]json.RawMessage) *SharedFixtureProcess {
	return &SharedFixtureProcess{
		sharedDir: sharedDir,
		state:     state,
	}
}

type ExportFixtureWindows = fixtureWindows

// ExportSelectGroups narrows o to --group and --skip-group.
func ExportSelectGroups(o *OverlayResult, only, skip []string) error {
	return o.selectGroups(only, skip)
}

// ExportSortDeferred orders targets as a scope dispatches its held-back suites.
func ExportSortDeferred(targets []SuiteTarget) []SuiteTarget {
	ds := make([]deferredTarget, len(targets))
	for i := range targets {
		ds[i] = deferredTarget{t: targets[i], idx: i}
	}
	sortDeferred(ds)
	out := make([]SuiteTarget, len(ds))
	for i := range ds {
		out[i] = ds[i].t
	}
	return out
}

var ExportLogSlowBuild = logSlowBuild
var ExportSlowBuildThreshold = slowBuildThreshold

// ExportLineWriterProgress reads the progress a line writer captured.
func ExportLineWriterProgress(w io.WriteCloser) FuzzProgress { return w.(*lineWriter).progress }

// ExportLineWriterDeadlineFail reports whether the target's only failure was
// the engine's deadline.
func ExportLineWriterDeadlineFail(w io.WriteCloser) bool { return w.(*lineWriter).deadlineFail }

var ExportSnapshotCrashers = snapshotCrashers
var ExportNewCrasherNames = newCrasherNames
var ExportExitCodeAfterDispatch = exitCodeAfterDispatch

// ExportApplyDeadlineFailure applies a run's deadline failure under the given
// --timeout.
func ExportApplyDeadlineFailure(c *OutputCollector, result *PipelineResult, globalTimeout time.Duration, dispatchErr error, running []CensusCase) {
	applyDeadlineFailure(c, result, PipelineConfig{GlobalTimeout: globalTimeout}, dispatchErr, running)
}

var ExportUnitNames = unitNames

// ExportRunning indexes the stream written in chunks and lists the units it
// leaves running.
func ExportRunning(chunks ...string) []CensusCase {
	var x verdictIndex
	for _, c := range chunks {
		_, _ = io.WriteString(&x, c)
	}
	return x.runningUnits()
}

// ExportBookDeadline writes stream through a collector's JSON writer, books the
// deadline over it and returns the named units and the booked events.
func ExportBookDeadline(mode RunMode, stream string, dispatchErr error) (running []CensusCase, booked string) {
	var stdout, errw bytes.Buffer
	c := NewOutputCollector(mode, false, WithWriters(&stdout, &errw))
	_, _ = io.WriteString(c.jsonWriter(), stream)
	target := &stdout
	if mode == RunCaptureJSON {
		target = &c.captured
	}
	before := target.Len()
	running = c.bookDeadline(dispatchErr)
	return running, target.String()[before:]
}

// ExportBookDeadlineOn books the deadline on a collector that recorded results.
var ExportBookDeadlineOn = (*OutputCollector).bookDeadline

// ExportExecuted indexes the stream written in chunks and lists the test
// methods, fuzz wrappers and benchmarks it settles.
func ExportExecuted(chunks ...string) (tests, fuzz, benchmarks []CensusCase) {
	var x verdictIndex
	for _, c := range chunks {
		_, _ = io.WriteString(&x, c)
	}
	return x.executedTests(), x.executedFuzz(), x.benchOrder
}

// ExportCensus writes stream through a collector's JSON writer, takes the
// census over it and returns the exit code, stderr and the booked events.
func ExportCensus(mode RunMode, stream string, declared DeclaredUnits, goTestArgs []string, bench bool, code int, dispatchErr error) (exit int, stderr, booked string) { //nolint:gocritic // hugeParam: test hook
	var stdout, errw bytes.Buffer
	c := NewOutputCollector(mode, false, WithWriters(&stdout, &errw))
	_, _ = io.WriteString(c.jsonWriter(), stream)
	target := &stdout
	if mode == RunCaptureJSON {
		target = &c.captured
	}
	before := target.Len()
	exit = c.takeCensus(PipelineConfig{GoTestArgs: goTestArgs, Bench: bench}, declared, code, dispatchErr)
	return exit, errw.String(), target.String()[before:]
}

// ExportNewStateProcess builds a shared fixture process handle that holds
// state without a subprocess behind it, for the state-file tests.
func ExportNewStateProcess(sharedDir string, state map[string]json.RawMessage) *SharedFixtureProcess {
	return &SharedFixtureProcess{sharedDir: sharedDir, state: state}
}

// ExportNewFailureLog, ExportTakeFailures and ExportFailureLogLimit expose
// what the runner keeps of the shared fixture process's stderr.
func ExportNewFailureLog(out io.Writer) io.Writer { return newFailureLog(out) }

func ExportTakeFailures(w io.Writer) []string { return w.(*failureLog).take() }

const ExportFailureLogLimit = failureLogLimit

// ExportBenchCollector marks c as a bench run's collector.
func ExportBenchCollector(c *OutputCollector) { c.bench = true }

// ExportReadProtocol feeds a protocol stream to a process that has no
// subprocess behind it and returns it once the stream ended.
func ExportReadProtocol(stream io.Reader) *SharedFixtureProcess {
	p := &SharedFixtureProcess{
		ready:    map[string]chan struct{}{},
		state:    map[string]json.RawMessage{},
		allDone:  make(chan struct{}),
		failures: newFailureLog(io.Discard),
	}
	p.readProtocol(stream)
	return p
}

// ExportSetSetupReportFloor shrinks the wait before the first reported budget
// and returns what restores it.
func ExportSetSetupReportFloor(d time.Duration) (restore func()) {
	was := setupReportFloor
	setupReportFloor = d
	return func() { setupReportFloor = was }
}

// ExportNewSilentProcess is a process that never reports: no budget, no
// state, no end of setup.
func ExportNewSilentProcess() *SharedFixtureProcess {
	return &SharedFixtureProcess{
		state:      map[string]json.RawMessage{},
		allDone:    make(chan struct{}),
		budgetSeen: make(chan struct{}, 1),
		setupBegan: time.Now(),
	}
}

// ExportResulted counts the units whose result line the collector still keeps.
func ExportResulted(c *OutputCollector) int { return len(c.verdicts.resulted) }

var ExportApplyInterrupt = applyInterrupt

// ExportTrack tracks tree as a live tree rooted at root, stopped once more by
// again (nil: an interrupt), for Abandon, until the returned func untracks it.
func ExportTrack(tree *proctree.Tree, root int, again func() error, done <-chan struct{}) (stop func()) {
	track(tree, liveTree{root: root, again: again, done: done})
	return func() { untrack(tree) }
}

// ExportCountSuiteStarts counts suite process starts, calling after with the
// count once each start returns, and returns what restores the real start.
func ExportCountSuiteStarts(after func(n int)) (restore func()) {
	var n atomic.Int32
	suiteStart = func(ctx context.Context, target SuiteTarget, env []string, test2json bool) (startedSuite, error) { //nolint:gocritic // hugeParam: as startSuite
		s, err := startSuite(ctx, target, env, test2json)
		after(int(n.Add(1)))
		return s, err
	}
	return func() { suiteStart = startSuite }
}

// ExportFailSuiteStarts makes every suite start fail with err, calling each
// per start, and returns what restores the real start.
func ExportFailSuiteStarts(err error, each func()) (restore func()) {
	suiteStart = func(context.Context, SuiteTarget, []string, bool) (startedSuite, error) {
		each()
		return startedSuite{}, err
	}
	return func() { suiteStart = startSuite }
}

// ExportStubOwnerIdentity stands in ns for this process's pid namespace and
// boot for its boot, and returns what restores the real readers.
func ExportStubOwnerIdentity(ns, boot func() string) (restore func()) {
	selfPIDNamespace, selfBoot = ns, boot
	return func() { selfPIDNamespace, selfBoot = pidNamespace, bootID }
}

// ExportExited is closed once the shared fixture process has exited.
func ExportExited(p *SharedFixtureProcess) <-chan struct{} { return p.done }

var ExportSanitizePkgName = sanitizePkgName

// ExportLockOpened runs then each time a lock file is opened, before it is
// locked, until the returned call restores the default.
func ExportLockOpened(then func(path string)) (restore func()) {
	prev := lockOpened
	lockOpened = then
	return func() { lockOpened = prev }
}

// ExportAfterScan runs then between the binary cache sweep's scan and its
// evictions, until the returned call restores the default.
func ExportAfterScan(then func()) (restore func()) {
	prev := afterScan
	afterScan = then
	return func() { afterScan = prev }
}

// ExportKeyEmptied runs then between the check that a key dir is empty and
// its removal, until the returned call restores the default.
func ExportKeyEmptied(then func(dir string)) (restore func()) {
	prev := keyEmptied
	keyEmptied = then
	return func() { keyEmptied = prev }
}

var ExportSweepBinaryCache = sweepBinaryCache

// ExportTeardownVerdict is teardownVerdict with the reports its error carries.
func ExportTeardownVerdict(teardownErr error, reports []string) ([]string, error) {
	err := teardownVerdict(teardownErr, reports)
	if f := (*fixtureFailure)(nil); errors.As(err, &f) {
		return f.reports, err
	}
	return nil, err
}

// ExportCommand sends one window verb as is, past what TeardownKeys leaves
// to the final teardown.
func ExportCommand(ctx context.Context, p *SharedFixtureProcess, verb string, keys []string, timeout time.Duration) error {
	return p.command(ctx, verb, keys, timeout)
}

// ExportGroupRuns books targets under a fresh group clock, runs those named
// in ran, and returns what the clock reports.
func ExportGroupRuns(targets []SuiteTarget, ran map[string]bool) []GroupRun {
	var c groupClock
	c.count(targets)
	for i := range targets {
		if ran[targets[i].SuiteName] {
			c.run(targets[i].Group, func() {})
		}
	}
	return c.runs()
}

// WriteGate is the gate a collector's stream is written through; a test
// gives its collector one of its own, since the run's is taken for good.
type WriteGate = writeGate

// WithWriteGate writes the collector's stream through g.
func WithWriteGate(g *WriteGate) OutputOption {
	return func(c *OutputCollector) { c.gate = g }
}

// EndWritesOf ends the writes through g, as Exit does through the run's gate.
func EndWritesOf(g *WriteGate, bound time.Duration, hurry <-chan struct{}) bool {
	return g.end(bound, hurry)
}

// ExportExitGate is the run's gate, which Exit takes for good.
var ExportExitGate = &exitGate

// ExportObserveExit has Exit end the process by observe instead of os.Exit,
// until the returned func restores it.
func ExportObserveExit(observe func(code int)) (restore func()) {
	osExit = observe
	return func() { osExit = os.Exit }
}

// ResumeWrites releases a gate EndWritesOf took.
func ResumeWrites(g *WriteGate) { g.mu.Unlock() }

// WriteEvents writes p to the collector's stream in one write, as a run's
// own failure is booked.
func WriteEvents(c *OutputCollector, p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = c.jsonTarget().Write(p)
}

// ExportOnConverterStart hands each converter a relay starts to seen, until
// the returned call restores the default.
func ExportOnConverterStart(seen func(*os.Process)) (restore func()) {
	prev := converterStarted
	converterStarted = seen
	return func() { converterStarted = prev }
}

// profileflags.go
var ExportSharedProfileFlag = sharedProfileFlag

// ExportHoldToTests holds the collector's text run to the tests it runs.
func ExportHoldToTests(c *OutputCollector) { c.holdToTests = true }

// compile.go
var ExportCompileError = compileError

// ExportBookDeadlineCut writes stream through a -json collector as the output
// of one suite process, cut short by the run's context or not, books the
// deadline over it and returns the named units and the booked events.
func ExportBookDeadlineCut(stream string, cutShort bool) (running []CensusCase, booked string) {
	var stdout, errw bytes.Buffer
	c := NewOutputCollector(RunStreamJSON, false, WithWriters(&stdout, &errw))
	_, _ = io.WriteString(c.jsonWriter(), stream)
	c.verdicts.endSuite(cutShort)
	before := stdout.Len()
	running = c.bookDeadline(context.DeadlineExceeded)
	return running, stdout.String()[before:]
}

var ExportMayRunNothing = mayRunNothing

func WithWriters(stdout, stderr io.Writer) OutputOption {
	return func(c *OutputCollector) {
		c.stdout = stdout
		c.stderr = stderr
	}
}

// State returns a snapshot of the accumulated state for the given keys.
func (p *SharedFixtureProcess) State(keys []string) map[string]json.RawMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make(map[string]json.RawMessage, len(keys))
	for _, k := range keys {
		if v, ok := p.state[k]; ok {
			result[k] = v
		}
	}
	return result
}

func (p *ManagedProcess) Done() <-chan struct{} { return p.done }

// DispatchEvent is one step of a run's dispatch.
type DispatchEvent = dispatchEvent

// ExportObserveDispatch hands every dispatch step to fn until the returned
// call restores the default.
func ExportObserveDispatch(fn func(DispatchEvent)) (restore func()) {
	prev := observeDispatch
	observeDispatch = fn
	return func() { observeDispatch = prev }
}

// plan.go
var ExportPlanRun = planRun

// ExportPlanFromResults projects results onto the per-package maps a run
// once read, and plans from those.
func ExportPlanFromResults(results gotestgen.GenerateResults) *RunPlan {
	return mapsOf(results).plan()
}

// ExportTestRun names the suites a test run of plan dispatches, in plan
// order; nil for none.
func ExportTestRun(plan *RunPlan, run, skip string, only, skipGroups []string) []string {
	var names []string
	for _, s := range plan.testRun(selection{Run: run, Skip: skip, Groups: newGroupSelection(only, skipGroups)}) {
		names = append(names, s.Name)
	}
	return names
}

// ExportScope is one planned scope: its group and its two phases, package →
// test function names.
type ExportScope struct {
	Group      string
	Bulk, Tail map[string][]string
}

func exportPhase(phase map[string][]*PlannedSuite) map[string][]string {
	out := map[string][]string{}
	for pkg, suites := range phase {
		for _, s := range suites {
			out[pkg] = append(out[pkg], s.TestFunc())
		}
	}
	return out
}

// phaseSuites turns package → test function names back into o's planned
// suites; a name o does not plan reads no keys.
func phaseSuites(o *OverlayResult, phase map[string][]string) []*PlannedSuite {
	var out []*PlannedSuite
	for pkg, names := range phase {
		for _, name := range names {
			out = append(out, o.plan().Suite(pkg, strings.TrimPrefix(name, "Test")))
		}
	}
	return out
}

func overlaySelection(o *OverlayResult, run, skip string) selection {
	return selection{Run: run, Skip: skip, Groups: o.groups}
}

// ExportPlanSuiteScopes plans o's scopes under -run and -skip.
func ExportPlanSuiteScopes(o *OverlayResult, run, skip string) []ExportScope {
	var out []ExportScope
	for _, sc := range planSuiteScopes(o.plan(), overlaySelection(o, run, skip)) {
		out = append(out, ExportScope{Group: sc.Group, Bulk: exportPhase(sc.Bulk), Tail: exportPhase(sc.Tail)})
	}
	return out
}

// ExportPlanSuitePhases splits the suites of the given maps, all of the
// default group, into the bulk and the tail.
func ExportPlanSuitePhases(suitesByPkg map[string][]string, exclusiveByPkg map[string]map[string]bool, fuzzFuncsByPkg map[string]map[string][]string, testlessByPkg map[string]map[string]bool, run, skip string) (bulk, tail map[string][]string) {
	o := &OverlayResult{Plan: planMaps{SuitesByPkg: suitesByPkg, ExclusiveSuitesByPkg: exclusiveByPkg, FuzzFuncsByPkg: fuzzFuncsByPkg, TestlessSuitesByPkg: testlessByPkg}.plan()}
	scopes := ExportPlanSuiteScopes(o, run, skip)
	if len(scopes) == 0 {
		return map[string][]string{}, map[string][]string{}
	}
	return scopes[0].Bulk, scopes[0].Tail
}

// ExportAliveFixtureKeys closes, over o's fixtures, the keys o's suites in
// phase read.
func ExportAliveFixtureKeys(o *OverlayResult, phase map[string][]string) map[string]bool {
	return aliveFixtureKeys(phaseSuites(o, phase), o.SharedFixtures)
}

// ExportPlanFixtureWindows plans o's fixture windows under -run and -skip.
func ExportPlanFixtureWindows(o *OverlayResult, run, skip string) fixtureWindows {
	return planFixtureWindows(o.plan(), o.SharedFixtures, overlaySelection(o, run, skip))
}

// ExportPlanBenchFixtureWindows plans o's bench windows under -run, -bench
// and -skip.
func ExportPlanBenchFixtureWindows(o *OverlayResult, run, bench, skip string) fixtureWindows {
	sel := overlaySelection(o, run, skip)
	sel.Bench = bench
	return planBenchFixtureWindows(o.plan(), o.SharedFixtures, sel)
}

// ExportBenchSlotPlan plans the windows of bench targets in dispatch order,
// each target naming one of o's suites by its identifier.
func ExportBenchSlotPlan(o *OverlayResult, targets []SuiteTarget) (needs, laterNeeds []map[string]bool) {
	slots := make([]*PlannedSuite, len(targets))
	for i := range targets {
		slots[i] = o.plan().Suite(targets[i].Package, targets[i].SuiteName)
	}
	return benchSlotPlan(slots, o.SharedFixtures)
}

var ExportSuiteTargets = suiteTargets

// fixturesession.go

// ExportNewFixtureSession is a session whose up-front phase is over.
var ExportNewFixtureSession = settledFixtureSession

// ExportSessionEnv is the environment f hands a suite reading keys.
func ExportSessionEnv(f *fixtureSession, base []string, pkg, suite string, keys []string) ([]string, error) {
	return f.env(base, pkg, suite, keys)
}

// ExportSessionMissing names the first of keys f has not up.
func ExportSessionMissing(f *fixtureSession, keys []string) string { return f.missing(keys) }

// dispatch.go

// ExportSession is a run's fixture session.
type ExportSession = *fixtureSession

// ExportSuiteMode is how a suite gets to run.
type ExportSuiteMode = suiteMode

const (
	ExportStreamed = streamed
	ExportDeferred = deferred
	ExportAlone    = alone
)

// ExportNewStreamRun is a streaming run of plan booking into c, its fixtures
// in f, with slots dispatch slots. It ends with ctx.
func ExportNewStreamRun(ctx context.Context, plan *RunPlan, c *OutputCollector, f *fixtureSession, n int) *streamRun {
	ctx, cancel := context.WithCancel(ctx)
	return &streamRun{
		ctx: ctx, cancel: cancel, plan: plan, collector: c, fixtures: f,
		slots:    newSlots(n),
		ledger:   newFixtureLedger(nil, nil),
		releaser: startFixtureReleaser(ctx, func() *SharedFixtureProcess { return nil }),
	}
}

// ExportRunSuite runs the suite of r's first package named by its test
// function, in mode, at index 0.
func ExportRunSuite(r *streamRun, suite string, mode suiteMode) {
	r.runSuite(SuiteTarget{SuiteSpec: SuiteSpec{Package: r.plan.Packages[0].Path, SuiteName: suite}}, 0, mode, nil)
}

// ExportNewUpProcess is a process without a subprocess behind it whose every
// state key came up.
func ExportNewUpProcess(sharedDir string, state map[string]json.RawMessage) *SharedFixtureProcess {
	p := &SharedFixtureProcess{sharedDir: sharedDir, state: state, ready: map[string]chan struct{}{}}
	for key := range state {
		ch := make(chan struct{})
		close(ch)
		p.ready[key] = ch
	}
	return p
}

// BuildSuiteTargets builds the targets of the suites the maps declare that
// -run selects and -skip does not drop whole, for each compiled package.
func BuildSuiteTargets(compiled []CompileResult, suitesByPkg map[string][]string, dirsByPkg map[string]string, fuzzFuncsByPkg map[string]map[string][]string, testlessByPkg map[string]map[string]bool, exclusiveByPkg map[string]map[string]bool, runFlags []string, userRunFilter, userSkipFilter string) []SuiteTarget {
	plan := planMaps{SuitesByPkg: suitesByPkg, DirsByPkg: dirsByPkg, FuzzFuncsByPkg: fuzzFuncsByPkg, TestlessSuitesByPkg: testlessByPkg, ExclusiveSuitesByPkg: exclusiveByPkg}.plan()
	selected := byPackage(plan.testRun(selection{Run: userRunFilter, Skip: userSkipFilter}))
	var targets []SuiteTarget
	for _, cr := range compiled {
		targets = append(targets, suiteTargets(cr, selected[cr.Package], runFlags, userRunFilter)...)
	}
	return targets
}

// BuildBenchTargets builds the bench targets of the suites benchesByPkg
// declares that -run, -bench and -skip select, for each compiled package.
func BuildBenchTargets(compiled []CompileResult, benchesByPkg map[string][]string, dirsByPkg map[string]string, runFlags []string, userRunFilter, userBenchFilter, userSkipFilter string) []SuiteTarget {
	plan := planMaps{BenchesByPkg: benchesByPkg, DirsByPkg: dirsByPkg}.plan()
	return benchTargets(compiled, plan.benchRun(selection{Run: userRunFilter, Bench: userBenchFilter, Skip: userSkipFilter}), runFlags, userBenchFilter)
}

// ExportPlannedOverlay is an overlay holding fixtures and a plan of suites,
// each suite's package created on first mention with a generated harness.
func ExportPlannedOverlay(fixtures []gotestgen.SharedFixtureInfo, suites ...PlannedSuite) *OverlayResult { //nolint:gocritic // hugeParam: test literals
	p := &RunPlan{}
	at := map[string]int{}
	for _, s := range suites {
		i, ok := at[s.Package]
		if !ok {
			i = len(p.Packages)
			at[s.Package] = i
			p.Packages = append(p.Packages, PlannedPackage{Path: s.Package, Dir: s.Dir, Harness: true})
		}
		p.Packages[i].Suites = append(p.Packages[i].Suites, s)
	}
	p.index()
	return &OverlayResult{SharedFixtures: fixtures, Plan: p}
}

// planMaps are the per-package maps a run once read its suites from. Tests
// that state suites as maps plan through them.
type planMaps struct {
	SuitePackages, NoSuitePackages []string
	StdlibTestsByPkg               map[string]int
	SuitesByPkg                    map[string][]string
	ExclusiveSuitesByPkg           map[string]map[string]bool
	GroupsByPkg                    map[string]map[string]string
	TestlessSuitesByPkg            map[string]map[string]bool
	BenchesByPkg                   map[string][]string
	FuzzFuncsByPkg                 map[string]map[string][]string
	FuzzParamsByFunc               map[string]map[string][]string
	FuzzDecodedByFunc              map[string]map[string]bool
	DirsByPkg                      map[string]string
	SkippedSuitesByPkg             map[string][]string
	SuiteRequiredSharedFixtureKeys map[string]map[string][]string // package → test function → keys
}

// mapsOf projects results onto the maps, as runs once did.
func mapsOf(results gotestgen.GenerateResults) planMaps {
	m := planMaps{
		StdlibTestsByPkg: map[string]int{}, SuitesByPkg: map[string][]string{}, ExclusiveSuitesByPkg: map[string]map[string]bool{},
		GroupsByPkg: map[string]map[string]string{}, TestlessSuitesByPkg: map[string]map[string]bool{}, BenchesByPkg: map[string][]string{},
		FuzzFuncsByPkg: map[string]map[string][]string{}, FuzzParamsByFunc: map[string]map[string][]string{}, FuzzDecodedByFunc: map[string]map[string]bool{},
		DirsByPkg: map[string]string{}, SkippedSuitesByPkg: map[string][]string{}, SuiteRequiredSharedFixtureKeys: map[string]map[string][]string{},
	}
	set := func(names []string) map[string]bool {
		out := map[string]bool{}
		for _, n := range names {
			out[n] = true
		}
		return out
	}
	for _, r := range results {
		if len(r.PTest) > 0 || len(r.PXTest) > 0 {
			m.SuitePackages = append(m.SuitePackages, r.PkgPath)
		} else {
			m.NoSuitePackages = append(m.NoSuitePackages, r.PkgPath)
		}
		if r.StdlibTestCount > 0 {
			m.StdlibTestsByPkg[r.PkgPath] = r.StdlibTestCount
		}
		if r.AbsPath != "" {
			m.DirsByPkg[r.PkgPath] = r.AbsPath
		}
		if len(r.SkippedSuiteNames) > 0 {
			m.SkippedSuitesByPkg[r.PkgPath] = r.SkippedSuiteNames
		}
		m.SuitesByPkg[r.PkgPath] = r.SuiteNames
		m.BenchesByPkg[r.PkgPath] = r.BenchSuiteNames
		m.FuzzFuncsByPkg[r.PkgPath] = r.FuzzFuncsBySuite
		m.FuzzParamsByFunc[r.PkgPath] = r.FuzzParamsByFunc
		m.FuzzDecodedByFunc[r.PkgPath] = r.FuzzDecodedFuncs
		m.ExclusiveSuitesByPkg[r.PkgPath] = set(r.ExclusiveSuiteNames)
		m.TestlessSuitesByPkg[r.PkgPath] = set(r.TestlessSuiteNames)
		m.GroupsByPkg[r.PkgPath] = r.GroupBySuite
		byFunc := map[string][]string{}
		for id, keys := range r.SuiteRequiredSharedFixtureKeys {
			byFunc["Test"+id] = keys
		}
		m.SuiteRequiredSharedFixtureKeys[r.PkgPath] = byFunc
	}
	return m
}

// plan rebuilds a plan from the maps. Packages are SuitePackages then
// NoSuitePackages, or the sorted union of the maps' keys when neither is set.
func (m planMaps) plan() *RunPlan { //nolint:gocritic // hugeParam: test helper
	harness := map[string]bool{}
	for _, pkg := range m.SuitePackages {
		harness[pkg] = true
	}
	paths := slices.Concat(m.SuitePackages, m.NoSuitePackages)
	if len(paths) == 0 {
		set := map[string]bool{}
		addKeys(set, m.SuitesByPkg)
		addKeys(set, m.BenchesByPkg)
		addKeys(set, m.DirsByPkg)
		addKeys(set, m.StdlibTestsByPkg)
		addKeys(set, m.SkippedSuitesByPkg)
		addKeys(set, m.FuzzFuncsByPkg)
		addKeys(set, m.GroupsByPkg)
		addKeys(set, m.ExclusiveSuitesByPkg)
		addKeys(set, m.TestlessSuitesByPkg)
		addKeys(set, m.SuiteRequiredSharedFixtureKeys)
		paths = slices.Sorted(maps.Keys(set))
	}
	p := &RunPlan{Packages: make([]PlannedPackage, 0, len(paths))}
	for _, path := range paths {
		pkg := PlannedPackage{
			Path:        path,
			Dir:         m.DirsByPkg[path],
			Harness:     harness[path],
			StdlibTests: m.StdlibTestsByPkg[path],
			Skipped:     m.SkippedSuitesByPkg[path],
		}
		benches := m.BenchesByPkg[path]
		suite := func(name string, tests bool) PlannedSuite {
			s := PlannedSuite{
				Package:    path,
				Dir:        m.DirsByPkg[path],
				Name:       name,
				Tests:      tests,
				Bench:      slices.Contains(benches, name),
				SharedKeys: sortedCopy(m.SuiteRequiredSharedFixtureKeys[path]["Test"+name]),
			}
			if tests {
				s.Testless = m.TestlessSuitesByPkg[path][name]
				s.Exclusive = m.ExclusiveSuitesByPkg[path][name]
				s.Group = m.GroupsByPkg[path][name]
			}
			for _, fn := range m.FuzzFuncsByPkg[path][name] {
				s.FuzzFuncs = append(s.FuzzFuncs, PlannedFuzz{Func: fn, Params: m.FuzzParamsByFunc[path][fn], Decoded: m.FuzzDecodedByFunc[path][fn]})
			}
			return s
		}
		suites := m.SuitesByPkg[path]
		for _, name := range suites {
			pkg.Suites = append(pkg.Suites, suite(name, true))
		}
		for _, name := range benches {
			if !slices.Contains(suites, name) {
				pkg.Suites = append(pkg.Suites, suite(name, false))
			}
		}
		p.Packages = append(p.Packages, pkg)
	}
	p.index()
	return p
}

func addKeys[V any](set map[string]bool, m map[string]V) {
	for k := range m {
		set[k] = true
	}
}

// ExportBooked counts the suite results c has booked for pkg.
func ExportBooked(c *OutputCollector, pkg string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pkgs[pkg].completed
}

var ExportSpanOf = spanOf

var ExportBuildSet = buildSet

var ExportTestBinaryName = testBinaryName
var ExportCompileGroups = compileGroups

var ExportEventPackage = eventPackage

// BuildLog is what one go test -c -json call said, by package.
type BuildLog = buildLog

var ExportNewBuildLog = newBuildLog

func (l *buildLog) ExportFailed(pkg string) bool        { return l.failed(pkg) }
func (l *buildLog) ExportDiagnostics(pkg string) []byte { return l.diagnostics(pkg) }
func (l *buildLog) ExportShared() []byte                { return l.shared() }

var ExportLinkOrCopy = linkOrCopy

// StagedBinary is one package's slot in a group's stage dir.
type StagedBinary = stagedBinary

// ExportReadiness is what a slot holds.
type ExportReadiness = readiness

const (
	ExportNotReady  = notReady
	ExportConfirmed = confirmed
	ExportRebuilt   = rebuilt
)

var (
	ExportStageGroup  = stageGroup
	ExportBinaryReady = binaryReady
	ExportRenameSafe  = renameSafe
	ExportPublish     = publish
	ExportReportOrder = reportOrder
)

// ExportPlaced reports whether a cached binary was placed in b's slot.
func ExportPlaced(b *StagedBinary) bool { return b.placed != nil }

// ExportSetGoTestC replaces the go command the compile stage runs until
// restore.
func ExportSetGoTestC(fn func(ctx context.Context, args, env []string, stdout, stderr io.Writer) error) (restore func()) {
	prev := goTestC
	goTestC = fn
	return func() { goTestC = prev }
}

// ExportSetGoCacheDir makes dir the GOCACHE the compile stage reads until
// restore.
func ExportSetGoCacheDir(dir string) (restore func()) {
	prev := goCacheDir
	goCacheDir = func() string { return dir }
	return func() { goCacheDir = prev }
}

var ExportLinkFlags = linkFlags

var ExportEnvKeyFlags = envKeyFlags

// ExportSetHardLink replaces the hard link a placement makes until restore.
func ExportSetHardLink(fn func(oldname, newname string) error) (restore func()) {
	prev := hardLink
	hardLink = fn
	return func() { hardLink = prev }
}

// ExportDispatchedPackages is the packages a test run of o dispatches
// under -run, -skip and --skip-group.
func ExportDispatchedPackages(o *OverlayResult, run, skip string, skipGroups []string) map[string]bool {
	return dispatchedPackages(o.plan().testRun(selection{Run: run, Skip: skip, Groups: newGroupSelection(nil, skipGroups)}))
}

// ExportNewSlots, ExportQueue and ExportLeave drive the run's suite slots.
func ExportNewSlots(n int) *slots              { return newSlots(n) }
func ExportQueue(s *slots) chan struct{}       { return s.queue() }
func ExportLeave(s *slots, held chan struct{}) { s.leave(held) }

// ExportPackageResult is one suite result of a package, as a text run
// records it.
type ExportPackageResult struct {
	Pkg    string
	Result SuiteResult
}

// ExportTextCensus records results into a text run's collector in order,
// one package after another, takes the census and returns its exit code,
// its stderr, and what the run judged and failed.
func ExportTextCensus(results []ExportPackageResult, declared DeclaredUnits, cfg PipelineConfig) (exit int, stderr string, judged *protocol.RunResult, failed []protocol.FailedUnit) { //nolint:gocritic // hugeParam: test hook
	var stdout, errw bytes.Buffer
	c := NewOutputCollector(RunBatchText, false, WithWriters(&stdout, &errw))
	c.censusAside = censusAside(cfg, ClassifyGoTestArgs(cfg.GoTestArgs).RunFlags)
	count := map[string]int{}
	for i := range results {
		count[results[i].Pkg]++
	}
	for i := range results {
		if _, ok := c.pkgs[results[i].Pkg]; !ok {
			c.Register(results[i].Pkg, count[results[i].Pkg])
		}
	}
	seen := map[string]int{}
	for i := range results {
		c.RecordResult(results[i].Pkg, seen[results[i].Pkg], results[i].Result)
		seen[results[i].Pkg]++
	}
	c.Finalize()
	code := 0
	if c.AnyFailed() {
		code = 1
	}
	exit = c.takeCensus(cfg, declared, code, nil)
	judged, failed = c.judged()
	return exit, errw.String(), judged, failed
}

// ExportStreamFailures writes stream through a -json collector opened by a
// manifest and returns what it judged and the units it failed.
func ExportStreamFailures(stream string) (*protocol.RunResult, []protocol.FailedUnit) {
	c := NewOutputCollector(RunStreamJSON, false, WithWriters(io.Discard, io.Discard), WithManifest(&protocol.Manifest{}))
	_, _ = io.WriteString(c.jsonWriter(), stream)
	return c.judged()
}

var ExportRerunUnits = rerunUnits

var (
	ExportRecordPathIn  = recordPathIn
	ExportWriteRecordIn = writeRecordIn
	ExportReadRecordIn  = readRecordIn
)

// ExportVacuousInText is what a text run's process of pkg leaves vacuous,
// read from the counts it wrote and its stdout, and whether it counted.
func ExportVacuousInText(counts, stdout string) (vacuous []string, counted bool) {
	var x assertionIndex
	x.observeCounts("pkg", []byte(counts), []byte(stdout))
	for _, v := range x.vacuous {
		vacuous = append(vacuous, v.Path)
	}
	return vacuous, x.counted
}
