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

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// Ctrl-C while Main tears down lets the teardown finish, then the process
// ends by the signal, not with the tests' exit code.
func (s *StoppingTestSuite) TestCtrlCDuringTeardownEndsByTheSignal(t *gotest.T) {
	cmd := exec.Command(s.bin["quick"], "-test.run=TestQuickTestSuite") //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), "STOPPING_SLOW_TEARDOWN=1")
	tree, log := startTreeUntil(t, cmd, "STOPPING_LOG", "db tearing")

	gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
	err := wait(tree, cmd)

	gotest.Equal(t, []string{"db up", "db tearing", "db down"}, readEvents(log))
	gotest.True(t, stoppedFromOutside(err), "the process ends by the signal: %v", err)
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

// A suite process whose runner is gone has nobody left to announce a stop: a
// SIGTERM then counts, and the fixtures are released.
func (s *StoppingTestSuite) TestAnOrphanedSuiteHonoursASignal(t *gotest.T) {
	log := filepath.Join(t.TempDir(), "events")
	stop := filepath.Join(t.TempDir(), "stop") // never created
	// The shell starts the binary in the background, waits until it holds its
	// connection and exits: the binary's parent is gone, as when the CLI
	// crashed after starting its suites.
	sh := exec.Command("sh", "-c", `"$BIN" -test.run=TestHoldingTestSuite >/dev/null 2>&1 & echo $!; until grep -q running "$STOPPING_LOG" 2>/dev/null; do sleep 0.05; done`)
	sh.Env = append(ownEnv(), "BIN="+s.bin["hold"], "STOPPING_HOLD=1", "STOPPING_LOG="+log, protocol.EnvStopFile+"="+stop)
	out, err := sh.Output()
	gotest.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	gotest.NoError(t, err)
	for deadline := time.Now().Add(30 * time.Second); !slices.Contains(readEvents(log), "running"); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			gotest.Fail(t, "the orphaned binary never held its connection") //nolint:fail-guard // it must kill the process before failing
		}
	}

	gotest.NoError(t, syscall.Kill(pid, syscall.SIGTERM))
	for deadline := time.Now().Add(15 * time.Second); syscall.Kill(pid, 0) == nil; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			gotest.Fail(t, "the orphaned binary ignored SIGTERM") //nolint:fail-guard // it must kill the process before failing
		}
	}
	gotest.Equal(t, []string{"db up", "running", "pool down", "db down"}, readEvents(log))
}
