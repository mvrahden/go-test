//go:build unix

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// wrapper writes an -exec wrapper script into dir and returns its path.
func wrapper(t *gotest.T, dir, body string) string {
	path := filepath.Join(dir, "wrapper.sh")
	gotest.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700)) //nolint:gosec // G306: the script must be executable
	return path
}

// ParentGoneTestSuite: a suite process whose parent ends under the runner
// stops and tears down by itself.
type ParentGoneTestSuite struct {
	cli string
}

// SuiteConfig: cold, BeforeAll took 7.4s locally and a method 0.7s; both
// budgets hold twice a run seven times slower.
func (s *ParentGoneTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 2 * time.Minute}
}

func (s *ParentGoneTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *ParentGoneTestSuite) AfterAll(t *gotest.T) {}

func (s *ParentGoneTestSuite) TestTheParentIsKilled(t *gotest.T) {
	markerDir := t.TempDir()
	// The wrapper stays the suite's parent: sh waits for the command it runs.
	wrap := wrapper(t, t.TempDir(), `echo "wrapper=$$" >> "$GOTEST_CANARY_DIR/markers"
"$@"
`)
	cmd := commandOf(s.cli, "-json", "-exec", wrap, "../testdata/stopping/")
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := started(t, cmd, markerDir, "running", "wrapper=", "pid=")
	parent, err := strconv.Atoi(markerValue(awaitMarkers(markerDir, 0), "wrapper"))
	gotest.NoError(t, err)
	signalled(t, cmd, tree, syscall.Kill(parent, syscall.SIGKILL))
	markers := awaitMarkers(markerDir, 60*time.Second, "fixture.AfterAll")
	run := ended(cmd, tree, markerDir, 60*time.Second)
	pid, _ := strconv.Atoi(markerValue(run.Markers, "pid"))

	t.When("the parent of a suite process under the runner is killed while a method holds", func(w *gotest.T) {
		w.It("ends the holding method's context", func(it *gotest.T) {
			gotest.Contains(it, markers, "context ended", "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("the methods that have not started never run", func(it *gotest.T) {
			gotest.NotContains(it, markers, "second")
			gotest.NotContains(it, markers, "third")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
		w.It("leaves no suite process behind", func(it *gotest.T) {
			gotest.NotZero(it, pid, "markers: %v", run.Markers)
			gotest.True(it, gone(pid), "the suite process %d outlived the run", pid)
		})
	})
}

// commandOf is the CLI invocation with args, run from this package.
func commandOf(cli string, args ...string) *exec.Cmd {
	return exec.Command(cli, args...) //nolint:gosec // G204: binary built by this suite
}

// ForeignPidTestSuite: a cancelled run signals the pid a suite published only
// when that pid is in the suite's own process group.
type ForeignPidTestSuite struct {
	cli string
}

func (s *ForeignPidTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: setup ~14s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, SetupTimeout: 30 * time.Second}
}

func (s *ForeignPidTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *ForeignPidTestSuite) AfterAll(t *gotest.T) {}

type strangerCtx struct {
	marker   string
	stranger *exec.Cmd
	exited   chan struct{}
}

// BeforeEach starts a process in a group of its own that records a SIGTERM
// and answers a SIGWINCH.
func (s *ForeignPidTestSuite) BeforeEach(t *gotest.T) *strangerCtx {
	marker := filepath.Join(t.TempDir(), "stranger-got-term")
	cmd := exec.Command("/bin/sh", "-c", `trap 'touch "$0"; exit 0' TERM; trap 'touch "$0.probe"' WINCH; while :; do sleep 0.05; done`, marker)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	gotest.NoError(t, cmd.Start())
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	return &strangerCtx{marker: marker, stranger: cmd, exited: exited}
}

// settled returns once the stranger has acted on every signal sent before
// the call. The shell runs pending traps in signal order, so a SIGTERM
// already sent ends it before it answers the later SIGWINCH.
func (ctx *strangerCtx) settled(within time.Duration) bool {
	_ = ctx.stranger.Process.Signal(syscall.SIGWINCH)
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.exited:
			return true
		case <-time.After(20 * time.Millisecond):
		}
		if _, err := os.Stat(ctx.marker + ".probe"); err == nil {
			return true
		}
	}
	return false
}

func (s *ForeignPidTestSuite) AfterEach(t *gotest.T, ctx *strangerCtx) {
	_ = ctx.stranger.Process.Kill()
	<-ctx.exited
}

func (s *ForeignPidTestSuite) TestThePublishedPidIsAStranger(t *gotest.T, ctx *strangerCtx) {
	markerDir := t.TempDir()
	// The wrapper runs the suite, then puts the stranger's pid in the file
	// the suite published its own in, and records the signal its group gets.
	wrap := wrapper(t, t.TempDir(), `trap 'echo "group interrupted" >> "$GOTEST_CANARY_DIR/markers"' TERM INT
pidfile="$GOTEST_STOP_FILE.pid"
"$@" &
child=$!
while [ "$(cat "$pidfile" 2>/dev/null)" != "$child" ]; do kill -0 "$child" 2>/dev/null || exit 1; sleep 0.02; done
echo "$LCB_STRANGER" > "$pidfile"
echo "pid swapped" >> "$GOTEST_CANARY_DIR/markers"
while kill -0 "$child" 2>/dev/null; do wait "$child"; done
`)
	cmd := commandOf(s.cli, "-json", "-exec", wrap, "../testdata/stopping/")
	cmd.Env = testkit.ChildEnv(markerDir, "LCB_STRANGER="+strconv.Itoa(ctx.stranger.Process.Pid))
	tree := started(t, cmd, markerDir, "running", "pid swapped")
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	awaitMarkers(markerDir, 60*time.Second, "fixture.AfterAll")
	run := ended(cmd, tree, markerDir, 60*time.Second)
	gotest.True(t, ctx.settled(30*time.Second), "the stranger did not answer its probe")

	t.When("the run is interrupted and the pid the suite published names a process outside its group", func(w *gotest.T) {
		w.It("sends that process no signal", func(it *gotest.T) {
			_, err := os.Stat(ctx.marker)
			gotest.True(it, os.IsNotExist(err), "the stranger was sent SIGTERM")
		})
		w.It("interrupts the suite's process group instead", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "group interrupted", "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
	})
}
