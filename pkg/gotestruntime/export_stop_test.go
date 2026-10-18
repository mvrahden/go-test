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
