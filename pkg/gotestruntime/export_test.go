package gotestruntime

// Shims exposing the runtime's internals to the external ring-0 suites.

var (
	ExportRun                   = run
	ExportRunBeforeAllWithRetry = runBeforeAllWithRetry
	ExportSetupDAG              = setupDAG
	ExportComputeMaxDAGPath     = computeMaxDAGPath
	ExportComputeMaxTreePath    = computeMaxTreePath
	ExportCountMatching         = countMatching
)

// ExportNewNodeTracker returns an empty tracker of the kind the DAG setup expects.
func ExportNewNodeTracker() *nodeTracker {
	return &nodeTracker{succeeded: make(map[*FixtureNode]bool)}
}
