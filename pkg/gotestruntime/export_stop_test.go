package gotestruntime

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
)

// ExportSettle stops the run as a signal would and waits for it to finish
// on its own, giving a running method body grace; it reports whether the run
// finished, leaving the process alive either way. self's own body and suite
// are not waited for.
func ExportSettle(self testing.TB, stderr io.Writer, grace time.Duration) bool {
	settled, _ := ExportStop(self, stderr, grace, nil)
	return settled
}

// ExportStop is [ExportSettle] that, when the stop is forced, also releases
// the held fixtures as a forced stop does, reporting whether that finished;
// the process is left alive. Signals sent on again arrive as further stop
// signals would.
func ExportStop(self testing.TB, stderr io.Writer, grace time.Duration, again chan os.Signal) (settled, released bool) {
	return exportStop(self, stderr, grace, again, false)
}

// ExportStopFound is [ExportStop] for a stop the poll found in the stop
// file, before any signal asked for it.
func ExportStopFound(self testing.TB, stderr io.Writer, grace time.Duration, again chan os.Signal) (settled, released bool) {
	return exportStop(self, stderr, grace, again, true)
}

func exportStop(self testing.TB, stderr io.Writer, grace time.Duration, again chan os.Signal, found bool) (settled, released bool) {
	settleExcept = self
	openSuites.Add(-1)
	defer func() {
		openSuites.Add(1)
		settleExcept = nil
	}()
	w := &watcher{stderr: stderr, ch: again, unasked: found}
	releaseEnd, settled := w.settle(os.Interrupt, grace)
	if settled {
		return true, false
	}
	return false, w.release(releaseEnd)
}

// ExportWatchCount reports how many suites and holds the stop watcher serves.
func ExportWatchCount() int {
	watchMu.Lock()
	defer watchMu.Unlock()
	return watching
}

// ExportResetStop clears the process-wide stop, for tests that stop the run.
func ExportResetStop() {
	stopping.Store(false)
	runstate.Reset()
}

// ExportHoldSuiteOpen counts one more suite open, as OpenSuite does, until
// the returned call closes it.
func ExportHoldSuiteOpen() (closeIt func()) {
	openSuites.Add(1)
	return func() { openSuites.Add(-1) }
}

// ExportReleaseBound sets the bound a stop may spend releasing to d, which
// must be 10s or less, and the fixtures' teardown share of it to reserve,
// until the returned call restores both.
func ExportReleaseBound(d, reserve time.Duration) (restore func()) {
	budgetMu.Lock()
	was, wasReserve := maxBudget, maxReserve
	maxBudget, maxReserve = d, reserve
	budgetMu.Unlock()
	return func() {
		budgetMu.Lock()
		maxBudget, maxReserve = was, wasReserve
		budgetMu.Unlock()
	}
}

// ExportWatchStopFile starts a watcher as the runner's environment would,
// with path as the stop file, and returns the call that stops it. self is
// the test calling it, whose own body and suite a stop does not wait for.
func ExportWatchStopFile(self testing.TB, path string) (stop func()) {
	settleExcept = self
	openSuites.Add(-1)
	w := watchFile(io.Discard, path)
	return func() {
		w.close()
		openSuites.Add(1)
		settleExcept = nil
	}
}

// ExportAfterPublishPID runs then once the watcher has published its pid,
// until the returned call restores the default.
func ExportAfterPublishPID(then func()) (restore func()) {
	prev := publishPID
	publishPID = func(path string) {
		prev(path)
		then()
	}
	return func() { publishPID = prev }
}
