//go:build unix

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
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
		w.It("interrupts it before killing it", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "child interrupted")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
	})
}

// lateSignalWrapper writes an -exec wrapper that keeps the runner's signal
// from the suite: it publishes its own pid in place of the suite's, marking
// "pid swapped" once it has, and catches the signal sent to it. Caught, not
// ignored, so the suite keeps the signal's default action.
func lateSignalWrapper(t *gotest.T) string {
	path := filepath.Join(t.TempDir(), "wrapper.sh")
	script := `#!/bin/sh
trap ':' INT TERM
pidfile="$GOTEST_STOP_FILE.pid"
"$@" &
child=$!
while [ "$(cat "$pidfile" 2>/dev/null)" != "$child" ]; do kill -0 "$child" 2>/dev/null || exit 1; sleep 0.02; done
echo $$ > "$pidfile"
echo "pid swapped" >> "$GOTEST_CANARY_DIR/markers"
status=0
while kill -0 "$child" 2>/dev/null; do wait "$child"; status=$?; done
exit $status
`
	gotest.NoError(t, os.WriteFile(path, []byte(script), 0o700)) //nolint:gosec // G306: the script must be executable
	return path
}

// marked waits for the run to leave every marker in wants; one that never
// does is killed before the test fails.
func marked(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, markerDir string, wants ...string) {
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if !slices.ContainsFunc(wants, func(w string) bool { return !slices.Contains(got, w) }) {
			return
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never marked %v, got %v:\n%s%s", wants, got, cmd.Stdout, cmd.Stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
}

func (s *StopTestSuite) TestASignalAfterTheStopFileSettledTheRun(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-json", "-exec", lateSignalWrapper(t), "../testdata/stopping/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir, "STOPPING_LINGER=1")
	tree := holding(t, cmd, markerDir)
	marked(t, cmd, tree, markerDir, "pid swapped")
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	marked(t, cmd, tree, markerDir, "teardown done")
	signalled(t, cmd, tree, signalSuite(filepath.Join(markerDir, testkit.MarkersFile)))
	run := finished(cmd, tree, markerDir)

	t.When("the runner's signal reaches the suite only after its stop file settled the run", func(w *gotest.T) {
		w.It("ends with its tests' own status, every verdict in the stream", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "lingered", "the signal ended the process; stderr: %s", run.Stderr)
			for _, path := range []string{"TestStopTestSuite/TestHolds", "TestStopTestSuite", "-"} {
				gotest.NotEmpty(it, run.Verdict(path), "verdict of %s; stdout: %s", path, run.Stdout)
			}
		})
	})
}
