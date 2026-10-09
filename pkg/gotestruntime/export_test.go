package gotestruntime

import "context"

// Shims exposing the runtime's internals to the external ring-0 suites.

var (
	ExportRunBeforeAllWithRetry = runBeforeAllWithRetry
	ExportSetupDAG              = setupDAG
	ExportComputeMaxDAGPath     = computeMaxDAGPath
)

// ExportNewNodeTracker returns an empty tracker of the kind the DAG setup expects.
func ExportNewNodeTracker() *nodeTracker {
	return &nodeTracker{succeeded: make(map[*FixtureNode]bool)}
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

// ExportUseBudgetFile points the runtime at path for the budget it advertises,
// as the runner's environment does at start, and forgets earlier budgets.
func ExportUseBudgetFile(path string) {
	budgetMu.Lock()
	budgetFile = path
	maxBudget = 0
	budgetMu.Unlock()
}
