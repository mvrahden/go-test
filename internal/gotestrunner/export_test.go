package gotestrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
)

type ExportOverlayJSON = overlayJSON
type ExportFixtureStateEntry = fixtureStateEntry

var ExportCompileConcurrency = compileConcurrency
var ExportCompilePackage = compilePackage
var ExportBuildSuiteCmd = buildSuiteCmd
var ExportTest2JSONPath = test2jsonPath
var ExportResolveTest2JSON = resolveTest2JSON
var ExportTest2JSONArgv = test2jsonArgv
var ExportReadTeardownBudget = readTeardownBudget
var ExportBookBuildFailures = bookBuildFailures
var ExportBrokenPackageMessage = brokenPackageMessage
var ExportApplyTeardownFailure = applyTeardownFailure
var ExportSplitTopLevelOr = splitTopLevelOr
var ExportSuiteRunFilter = suiteRunFilter
var ExportAssignCoverProfiles = assignCoverProfiles
var ExportResolveSetupTimeout = resolveSetupTimeout

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

var ExportPlanFixtureWindows = planFixtureWindows
var ExportPlanBenchFixtureWindows = planBenchFixtureWindows
var ExportBenchSlotPlan = benchSlotPlan
var ExportPlanSuitePhases = planSuitePhases
var ExportAliveFixtureKeys = aliveFixtureKeys

var ExportSortTargetIndices = sortTargetIndices
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
		cmdResp:  make(chan string, 1),
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

// ExportRelay counts a suite run whose events are yet to reach the stream,
// until the returned func marks them passed on.
var ExportRelay = relay

// ExportTrack tracks tree as a live tree rooted at root, for Abandon, until
// the returned func untracks it.
func ExportTrack(tree *proctree.Tree, root int, stopFile string, done <-chan struct{}) (stop func()) {
	track(tree, liveTree{root: root, stopFile: stopFile, done: done})
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
