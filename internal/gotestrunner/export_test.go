package gotestrunner

import (
	"encoding/json"
	"os"
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

func ExportAutoDetectCI(cfg PipelineConfig) PipelineConfig {
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

var ExportSortTargetIndices = sortTargetIndices
