package gotestruntime

// Shims exposing the runtime's internals to the external ring-0 suites.

var (
	ExportRun                   = run
	ExportRunBeforeAllWithRetry = runBeforeAllWithRetry
	ExportSetupDAG              = setupDAG
	ExportComputeMaxDAGPath     = computeMaxDAGPath
	ExportComputeMaxTreePath    = computeMaxTreePath
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
