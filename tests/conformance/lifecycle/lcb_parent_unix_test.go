//go:build unix

package lifecycle_test

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParentGoneTestSuite: a suite process whose parent, the runner, ends stops
// and tears down by itself, its channel ended.
type ParentGoneTestSuite struct {
	cli string
}

// SuiteConfig: cold, BeforeAll took 7.4s locally and a method 0.7s; both
// budgets hold twice a run seven times slower.
func (s *ParentGoneTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 45 * time.Second}
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
	cmd := commandOf(s.cli, "-json", "../testdata/stopping/")
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := started(t, cmd, markerDir, "running", "pid=")
	// The runner is the suite's parent: killed, its end of the channel closes.
	signalled(t, cmd, tree, syscall.Kill(cmd.Process.Pid, syscall.SIGKILL))
	run := ended(cmd, tree, markerDir, 30*time.Second)
	markers := awaitMarkers(markerDir, 60*time.Second, "fixture.AfterAll")
	pid, _ := strconv.Atoi(markerValue(markers, "pid"))

	t.When("the parent of a suite process under the runner is killed while a method holds", func(w *gotest.T) {
		w.It("ends the holding method's context", func(it *gotest.T) {
			gotest.Contains(it, markers, "context ended", "stderr: %s", run.Stderr)
		})
		w.It("the methods that have not started never run", func(it *gotest.T) {
			gotest.NotContains(it, markers, "second")
			gotest.NotContains(it, markers, "third")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", markers)
		})
		w.It("leaves no suite process behind", func(it *gotest.T) {
			gotest.NotZero(it, pid, "markers: %v", markers)
			gotest.True(it, gone(pid), "the suite process %d outlived the run", pid)
		})
	})
}

// commandOf is the CLI invocation with args, run from this package.
func commandOf(cli string, args ...string) *exec.Cmd {
	return exec.Command(cli, args...) //nolint:gosec // G204: binary built by this suite
}
