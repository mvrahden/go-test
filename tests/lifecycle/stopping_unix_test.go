//go:build unix

package lifecycle_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Ctrl-C while a hold tears down lets the teardown finish; under plain go
// test the process then ends by the signal, which alone marks the run as
// interrupted there.
func (s *StoppingTestSuite) TestCtrlCDuringTeardownLetsItFinish(t *gotest.T) {
	cmd := exec.Command(s.bin["quick"], "-test.run=TestQuickTestSuite") //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), "STOPPING_SLOW_TEARDOWN=1")
	tree, log := startTreeUntil(t, cmd, "STOPPING_LOG", "db tearing")

	gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
	err := wait(tree, cmd)

	gotest.Equal(t, []string{"db up", "db tearing", "db down"}, readEvents(log))
	gotest.False(t, endedOnItsOwn(err), "under plain go test the process ends by the signal: %v", err)
}

// Under plain go test nothing but the process can mark the run interrupted:
// once the stopped suite has torn down, the process ends by the signal, and
// the suites after it never run.
func (s *StoppingTestSuite) TestCtrlCUnderPlainGoTestEndsTheRun(t *gotest.T) {
	cmd := exec.Command(s.bin["hold"], "-test.run=TestHoldingTestSuite|TestLaterTestSuite") //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), "STOPPING_HOLD=1")
	tree, log := startTreeUntil(t, cmd, "STOPPING_LOG", "running")
	gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
	err := waitWithin(t, tree, cmd, 20*time.Second)
	gotest.Equal(t, []string{"db up", "running", "pool down", "db down"}, readEvents(log))
	gotest.False(t, endedOnItsOwn(err), "under plain go test the process ends by the signal: %v", err)
}

// A teardown that runs a pipeline: the process handles SIGPIPE for itself
// without handing an ignored SIGPIPE to its children, whose writer would then
// never end.
func (s *StoppingTestSuite) TestATeardownPipelineFinishes(t *gotest.T) {
	log := filepath.Join(t.TempDir(), "events")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin["quick"], "-test.run=TestQuickTestSuite") //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), "STOPPING_PIPE=1", "STOPPING_LOG="+log)
	cmd.WaitDelay = 15 * time.Second
	began := time.Now()
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, string(out))
	gotest.Equal(t, []string{"db up", "pipe y", "db down"}, readEvents(log))
	gotest.Less(t, time.Since(began), 10*time.Second)
}

// A suite process whose runner is gone has nobody left to send a stop: the
// end of its channel then counts as one, and the fixtures are released.
func (s *StoppingTestSuite) TestAnOrphanedSuiteStopsAtItsChannelsEnd(t *gotest.T) {
	log := filepath.Join(t.TempDir(), "events")
	// The shell starts the binary in the background, waits until it holds its
	// connection and exits: the binary's parent is gone, as when the CLI
	// crashed after starting its suites. A group of its own: the orphan ends
	// its group once torn down.
	sh := exec.Command("sh", "-c", `"$BIN" -test.run=TestHoldingTestSuite >/dev/null 2>&1 & echo $!; until grep -q running "$STOPPING_LOG" 2>/dev/null; do sleep 0.05; done`)
	sh.Env = append(ownEnv(), "BIN="+s.bin["hold"], "STOPPING_HOLD=1", "STOPPING_LOG="+log)
	sh.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c := attach(t, sh)
	out, err := sh.Output()
	c.started()
	gotest.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	gotest.NoError(t, err)
	for deadline := time.Now().Add(30 * time.Second); !slices.Contains(readEvents(log), "running"); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			gotest.Fail(t, "the orphaned binary never held its connection") //nolint:fail-guard // it must kill the process before failing
		}
	}

	c.close() // the runner is gone
	for deadline := time.Now().Add(15 * time.Second); syscall.Kill(pid, 0) == nil; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			gotest.Fail(t, "the orphaned binary outlived its channel") //nolint:fail-guard // it must kill the process before failing
		}
	}
	gotest.Equal(t, []string{"db up", "running", "pool down", "db down"}, readEvents(log))
}

// The runner's force line with no stop under way, as when a second Ctrl-C
// overtakes the first's stop line, forces a stop of its own: the fixtures
// are released at once and the process ends by the signal, not after the
// method that ignores its context returns.
func (s *StoppingTestSuite) TestAForceWithNoStopForcesOne(t *gotest.T) {
	cmd, c := s.announced(t, "hold", "TestHoldingTestSuite", "STOPPING_HOLD=1", "STOPPING_IGNORE_CONTEXT=1")
	tree, log := startTreeUntil(t, cmd, "STOPPING_LOG", "running")
	c.started()
	began := time.Now()
	gotest.NoError(t, c.link.Force())
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	var err error
	select {
	case err = <-exited:
	case <-time.After(20 * time.Second):
		_ = tree.Kill()
		<-exited
		tree.Release()
		c.close()
		gotest.Fail(t, "a force with no stop under way left the process running") //nolint:fail-guard // the tree is killed before the test fails
	}
	tree.Release()
	c.close()
	gotest.Less(t, time.Since(began), 4*time.Second, "the force waited for the method")
	gotest.Equal(t, []string{"db up", "running", "pool down", "db down"}, readEvents(log))
	gotest.False(t, endedOnItsOwn(err), "the process ends by the signal: %v", err)
}
