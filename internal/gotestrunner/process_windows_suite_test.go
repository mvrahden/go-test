//go:build windows

package gotestrunner_test

import (
	"os"
	"os/exec"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// WindowsProcessTestSuite covers the liveness check that keeps a console
// control event from reaching every process on the console.
type WindowsProcessTestSuite struct{}

func (s *WindowsProcessTestSuite) TestProcessRunning(t *gotest.T) {
	t.It("reports the current process as running", func(it *gotest.T) {
		gotest.True(it, gotestrunner.ExportProcessRunning(os.Getpid()))
	})

	t.It("reports a process that exited as not running", func(it *gotest.T) {
		cmd := exec.Command(os.Getenv("ComSpec"), "/c", "exit 0") //nolint:gosec // G204: the system shell with a fixed argument
		gotest.NoError(it, cmd.Run())
		gotest.False(it, gotestrunner.ExportProcessRunning(cmd.Process.Pid))
	})
}
