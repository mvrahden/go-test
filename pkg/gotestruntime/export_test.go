package gotestruntime

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Shims exposing the runtime's internals to the external ring-0 suites.

var (
	ExportRunBeforeAllWithRetry = runBeforeAllWithRetry
	ExportSetupDAG              = setupDAG
	ExportComputeMaxDAGPath     = computeMaxDAGPath
)

// ExportNewNodeTracker returns an empty tracker of the kind the DAG setup expects.
func ExportNewNodeTracker() *nodeTracker {
	return &nodeTracker{}
}

// ExportRun sets the DAG up, runs the tests and tears it down, as a test
// binary does: exit 2 when setup fails, 1 when only the teardown did.
func ExportRun(runTests func() int, cfg MainConfig) int {
	dag, err := SetupFixtureDAG(context.Background(), cfg)
	if err != nil {
		return 2
	}
	code := runTests()
	if dag.Teardown() && code == 0 {
		code = 1
	}
	return code
}

// ExportBuildConfig runs a hold's build the way HoldFixtures does.
var ExportBuildConfig = buildConfig

// ExportWatching reports whether the stop watcher runs.
func ExportWatching() bool {
	watchMu.Lock()
	defer watchMu.Unlock()
	return active != nil
}

// ExportForgetBudgets forgets the budgets this process advertised so far.
func ExportForgetBudgets() {
	budgetMu.Lock()
	maxBudget = 0
	maxReserve = 0
	budgetMu.Unlock()
}

// ExportBudget is the teardown budget this process advertises.
func ExportBudget() time.Duration {
	budgetMu.Lock()
	defer budgetMu.Unlock()
	return maxBudget
}

// ExportUserFrameBelowHarness reads a goroutine traceback as the overrun
// verdict does.
var ExportUserFrameBelowHarness = userFrameBelowHarness

// ExportScaleSuite is c as GOTEST_TIMEOUT_SCALE scales it in this process.
func ExportScaleSuite(c gotest.SuiteConfig) gotest.SuiteConfig { return scaleSuite(c) }

// ExportScaleFixture is c as GOTEST_TIMEOUT_SCALE scales it in this process.
func ExportScaleFixture(c gotest.FixtureConfig) gotest.FixtureConfig {
	c.Timeout = scaleTimeout(c.Timeout)
	return c
}

// ExportSetTimeoutScale makes f the scale every duration is multiplied by and
// returns the restore of the reader it replaced.
func ExportSetTimeoutScale(f float64) (restore func()) {
	prev := timeoutScale
	timeoutScale = func() (float64, error) { return f, nil }
	return func() { timeoutScale = prev }
}
