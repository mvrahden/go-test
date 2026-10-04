package gotestruntime

import "context"

// Shims exposing the runtime's internals to the external ring-0 suites.

var (
	ExportRunBeforeAllWithRetry = runBeforeAllWithRetry
	ExportSetupDAG              = setupDAG
	ExportComputeMaxDAGPath     = computeMaxDAGPath
)

// ExportCountMatching predicts the countdown from -test.run and -test.skip alone.
func ExportCountMatching(testNames []string, run, skip string) int {
	return countMatching(testNames, testFilters{run: run, skip: skip})
}

// ExportCountMatchingFilters predicts the countdown from every selection flag.
func ExportCountMatchingFilters(testNames []string, run, skip, bench, fuzz string) int {
	return countMatching(testNames, testFilters{run: run, skip: skip, bench: bench, fuzz: fuzz})
}

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
