//go:build unix

package lifecycle_test

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// childOf reads the pid of the process the holding method started.
func childOf(markers []string) int {
	for _, m := range markers {
		if n, ok := strings.CutPrefix(m, "child="); ok {
			pid, _ := strconv.Atoi(n)
			return pid
		}
	}
	return 0
}

func (s *StopTestSuite) TestAStopEndsWhatATestStarted(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-json", "../testdata/stopping/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir, "STOPPING_CHILD=1")
	tree := holding(t, cmd, markerDir)
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	run := finished(cmd, tree, markerDir)
	child := childOf(run.Markers)
	gotest.NotZero(t, child, "markers: %v", run.Markers)
	gone := testkit.EndedWithin(child, 2*time.Second)
	if !gone {
		_ = syscall.Kill(child, syscall.SIGKILL)
	}

	t.When("the run is interrupted while a holding method has started a process of its own", func(w *gotest.T) {
		w.It("ends that process before gotest exits", func(it *gotest.T) {
			gotest.True(it, gone, "the process the test started outlived the run")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
	})
}
