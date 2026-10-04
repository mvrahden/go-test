package gotestruntime

import (
	"io"

	"github.com/mvrahden/go-test/internal/dying"
)

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

// ExportFinish applies the after-m.Run step to an exit code.
func ExportFinish(code int, stderr io.Writer) int { return finish(code, stderr) }

// ExportResetTeardowns empties the teardown registry and clears the dying mark.
func ExportResetTeardowns() {
	teardownMu.Lock()
	teardowns = nil
	teardownMu.Unlock()
	dying.Reset()
}

// ExportDying reports whether the process was marked as dying.
func ExportDying() bool { return dying.Marked() }
