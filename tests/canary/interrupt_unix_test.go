//go:build !windows

package canary_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// stoppedFromOutside reports whether a waited-for process ended by SIGINT or
// SIGTERM, or, when it started with the signal ignored and could not, by the
// status a shell gives for it.
func stoppedFromOutside(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return ws.Signal() == syscall.SIGINT || ws.Signal() == syscall.SIGTERM
	}
	return exit.ExitCode() == 128+int(syscall.SIGINT) || exit.ExitCode() == 128+int(syscall.SIGTERM)
}

// Outside the runner only Ctrl-C counts, SIGINT, which proctree never sends:
// it is delivered here as a terminal delivers it.
func (s *InterruptTestSuite) TestCtrlCReleasesTheFixture(t *gotest.T) {
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = ownEnv()
	tree, log := startTree(t, cmd)

	gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
	err := wait(tree, cmd)

	gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log))
	gotest.True(t, stoppedFromOutside(err), "the process still ends by the signal: %v", err)
}
