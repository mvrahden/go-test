package gotestruntime

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
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
	runstate.Reset()
	runState.Store(runNotStarted)
	releaseOnce = sync.Once{}
	budgetMu.Lock()
	maxBudget = 0
	budgetMu.Unlock()
}

// ExportDying reports whether the process was marked as dying.
func ExportDying() bool { return runstate.Dying() }

// ExportBeginStop moves an active run to stopping, as an accepted stop does.
func ExportBeginStop() bool { return beginStop() }

// ExportBeginSetup admits a fixture setup as SetupFixtureDAG does.
func ExportBeginSetup(ctx context.Context) (context.Context, func(), error) { return beginSetup(ctx) }

// ExportStartRun marks the run active without running it.
func ExportStartRun() { runState.Store(runActive) }

// ExportRecordBudget records an advertised teardown budget; ExportReleaseBound
// reads back how long a stop may then spend releasing.
func ExportRecordBudget(d time.Duration) time.Duration { return recordBudget(d) }
func ExportReleaseBound() time.Duration                { return releaseBound() }

// ExportRun sets the DAG up, runs the tests and tears it down, as a test
// binary does: exit 2 when setup fails, 1 when only the teardown did.
func ExportRun(runTests func() int, cfg MainConfig) int {
	dag, err := SetupFixtureDAG(context.Background(), cfg)
	if err != nil {
		return 2
	}
	RegisterTeardown(dag.Teardown)
	return finish(runTests(), io.Discard)
}

// ExportUseBudgetFile points the runtime at path for the budget it advertises,
// as the runner's environment does at startup, and forgets earlier budgets.
func ExportUseBudgetFile(path string) {
	budgetFile = path
	budgetMu.Lock()
	maxBudget = 0
	budgetMu.Unlock()
}
