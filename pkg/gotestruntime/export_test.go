package gotestruntime

import (
	"context"
	"io"

	"github.com/mvrahden/go-test/internal/dying"
)

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

// ExportFinish applies the after-m.Run step to an exit code.
func ExportFinish(code int, stderr io.Writer) int { return finish(code, stderr) }

// ExportRunTests runs run as Main and M run m.Run.
func ExportRunTests(run func() int, stderr io.Writer) int { return runTests(run, stderr) }

// ExportResetTeardowns empties the teardown registry and clears the dying mark.
func ExportResetTeardowns() {
	teardownMu.Lock()
	teardowns = nil
	teardownMu.Unlock()
	dying.Reset()
	runState.Store(runNotStarted)
}

// ExportDying reports whether the process was marked as dying.
func ExportDying() bool { return dying.Marked() }

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
