package gotestrunner

import (
	"encoding/json"
	"os"

	"github.com/mvrahden/go-test/internal/protocol"
)

type ExportOverlayJSON = overlayJSON
type ExportFixtureStateEntry = fixtureStateEntry

var ExportBuildSuiteCmd = buildSuiteCmd
var ExportReadTeardownBudget = readTeardownBudget
var ExportSplitTopLevelOr = splitTopLevelOr
var ExportSuiteRunFilter = suiteRunFilter
var ExportAssignCoverProfiles = assignCoverProfiles
var ExportBuildExtraEnv = buildExtraEnv
var ExportBuildBaseEnv = buildBaseEnv

func ExportAutoDetectCI(cfg PipelineConfig) PipelineConfig {
	if !cfg.CI && os.Getenv(protocol.EnvCI) == "" && os.Getenv("CI") != "" {
		cfg.CI = true
	}
	return cfg
}

var SetBuildProcessGroup = setBuildProcessGroup

func ExportNewSharedFixtureProcess(sharedDir string, state map[string]json.RawMessage) *SharedFixtureProcess {
	return &SharedFixtureProcess{
		sharedDir: sharedDir,
		state:     state,
	}
}
