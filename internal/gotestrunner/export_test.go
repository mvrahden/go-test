package gotestrunner

import (
	"encoding/json"
	"os"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
)

type ExportOverlayJSON = overlayJSON
type ExportFixtureStateEntry = fixtureStateEntry

var ExportCompileConcurrency = compileConcurrency
var ExportBuildSuiteCmd = buildSuiteCmd
var ExportReadTeardownBudget = readTeardownBudget
var ExportSplitTopLevelOr = splitTopLevelOr
var ExportSuiteRunFilter = suiteRunFilter
var ExportAssignCoverProfiles = assignCoverProfiles
var ExportResolveSetupTimeout = resolveSetupTimeout
var ExportBuildExtraEnv = buildExtraEnv
var ExportBuildBaseEnv = buildBaseEnv
var ExportOverlayContentHash = overlayContentHash
var ExportCacheRoot = cacheRoot
var ExportFilterPackageLevelEvents = filterPackageLevelEvents
var ExportIsPackageSummaryLine = isPackageSummaryLine

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
