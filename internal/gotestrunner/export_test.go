package gotestrunner

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
)

type ExportOverlayJSON = overlayJSON
type ExportFixtureStateEntry = fixtureStateEntry

var ExportCompileConcurrency = compileConcurrency
var ExportBuildSuiteCmd = buildSuiteCmd
var ExportReadTeardownBudget = readTeardownBudget
var ExportBookBuildFailures = bookBuildFailures
var ExportBrokenPackageMessage = brokenPackageMessage
var ExportApplyTeardownFailure = applyTeardownFailure
var ExportSplitTopLevelOr = splitTopLevelOr
var ExportSuiteRunFilter = suiteRunFilter
var ExportAssignCoverProfiles = assignCoverProfiles
var ExportResolveSetupTimeout = resolveSetupTimeout
var ExportBuildExtraEnv = buildExtraEnv
var ExportBuildBaseEnv = buildBaseEnv
var ExportOverlayContentHash = overlayContentHash
var ExportCacheRoot = cacheRoot
var ExportFilterPackageLevelEvents = filterPackageLevelEvents
var ExportIsPackageSummaryLine = protocol.IsPackageSummaryLine
var ResolveBenchParallelismForTest = resolveMaxParallel

// ExportProcessPID and ExportProcessDone let the teardown tests observe the
// shared fixture subprocess directly: whether it is still alive, and when it is
// finally reaped.
func ExportProcessPID(p *SharedFixtureProcess) int { return p.cmd.Process.Pid }

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

func ExportWriteOverlayCached(results gotestgen.GenerateResults, noCache bool) (string, error) {
	dir, _, err := writeOverlayCached(results, noCache)
	return dir, err
}

var SetBuildProcessGroup = setBuildProcessGroup

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

// ExportLineWriterProgress reads the progress a line writer captured.
func ExportLineWriterProgress(w io.WriteCloser) FuzzProgress { return w.(*lineWriter).progress }

var ExportSnapshotCrashers = snapshotCrashers
var ExportNewCrasherNames = newCrasherNames
var ExportExitCodeAfterDispatch = exitCodeAfterDispatch

// ExportApplyDeadlineFailure applies a run's deadline failure under the given
// --timeout.
func ExportApplyDeadlineFailure(result *PipelineResult, globalTimeout time.Duration, dispatchErr error) {
	applyDeadlineFailure(result, PipelineConfig{GlobalTimeout: globalTimeout}, dispatchErr)
}

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
