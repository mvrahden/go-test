package gotestruntime

import (
	"io"
	"os"
	"testing"
	"time"
)

// ExportSettle stops the run as a signal would and waits for it to finish
// on its own, giving a running method body grace; it reports whether the run
// finished, leaving the process alive either way. self is the test calling
// it, whose own body is not waited for.
func ExportSettle(self *testing.T, stderr io.Writer, grace time.Duration) bool {
	runningBodies.Add(-1)
	defer runningBodies.Add(1)
	w := &watcher{stderr: stderr}
	_, settled := w.settle(os.Interrupt, grace)
	return settled
}

// ExportWatchStopFile starts a watcher as the runner's environment would,
// with path as the stop file, and returns the call that stops it. self is
// the test calling it, whose own body a stop does not wait for.
func ExportWatchStopFile(self *testing.T, path string) (stop func()) {
	runningBodies.Add(-1)
	w := watchFile(io.Discard, path)
	return func() {
		w.close()
		runningBodies.Add(1)
	}
}
