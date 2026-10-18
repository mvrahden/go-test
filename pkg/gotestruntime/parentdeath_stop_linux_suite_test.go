//go:build linux

package gotestruntime_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParentDeathDuringStopTestSuite: the death signal a parent's exit sends is
// nobody's request, so landing while a requested stop tears down, it is not
// the second signal that abandons the teardown.
// Sequential: the scenario starts and kills processes.
type ParentDeathDuringStopTestSuite struct{}

func (s *ParentDeathDuringStopTestSuite) TestTheDeathSignalLeavesARequestedStopToFinish(t *gotest.T) {
	dir := t.TempDir()
	parent := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	parent.Env = append(os.Environ(), envGoneMode+"=parent", envGoneDir+"="+dir, envGoneSlow+"=1")
	gotest.NoError(t, parent.Start())
	pid := 0
	gotest.Eventually(t, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
		b, err := os.ReadFile(filepath.Join(dir, "suite.pid"))
		gotest.NoError(poll, err, "the suite process never opened its suite")
		pid, err = strconv.Atoi(string(b))
		gotest.NoError(poll, err)
	})
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }() // never leave it behind when the check fails

	// The stop the runner asks for: its stop file, then its signal.
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "suite.stop"), nil, 0o600))
	gotest.NoError(t, syscall.Kill(pid, syscall.SIGTERM))
	gotest.Eventually(t, 15*time.Second, 20*time.Millisecond, func(poll *gotest.R) {
		_, err := os.Stat(filepath.Join(dir, "stopped"))
		gotest.NoError(poll, err, "the stop did not end the method's context")
	})
	gotest.NoError(t, parent.Process.Kill())
	_ = parent.Wait()

	t.When("the parent dies while a stop it asked for runs AfterAll", func(w *gotest.T) {
		w.It("AfterAll runs to its end, then the process is gone", func(it *gotest.T) {
			gotest.Eventually(it, 15*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, processGone(pid), "the suite process outlived its teardown")
			})
			_, err := os.Stat(filepath.Join(dir, "torn-down"))
			gotest.NoError(it, err, "the death signal abandoned the teardown")
		})
	})
}
