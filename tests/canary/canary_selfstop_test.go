//go:build linux || darwin

package canary_test //nolint:fail-guard

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// markerValue is the value of the first key=value marker with key.
func markerValue(markers []string, key string) int {
	for _, m := range markers {
		if v, ok := strings.CutPrefix(m, key+"="); ok {
			n, _ := strconv.Atoi(v)
			return n
		}
	}
	return 0
}

// awaitMarker waits up to within for want in the markers file at path, the
// run's exit aside: a suite that stops by itself outlives the CLI.
func awaitMarker(path string, within time.Duration, want string) {
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if got, _ := testkit.ReadMarkers(path); slices.Contains(got, want) {
			return
		}
	}
}

// processGone reports whether pid, and any group it leads, is no process
// within a short grace period, and kills what is left when it is not. The
// suite process need not lead a group of its own, so the pid itself is probed.
func processGone(pid int) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) && errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
			return true
		}
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return false
}

// selfStopWithin is how long a suite that stops by itself may take to have
// released its fixture: well inside the holding method's 30s context, whose
// end would release it as well.
const selfStopWithin = 10 * time.Second

// selfStopped checks what a suite process that stopped by itself leaves: the
// holding method's context ended, no method that had not started ran,
// AfterAll ran before the fixture was released, and no suite process is left.
func selfStopped(t *gotest.T, name string, markers []string) {
	if !slices.Contains(markers, "context ended") {
		t.Errorf("%s: the holding method's context never ended: markers %v", name, markers)
	}
	if slices.Contains(markers, "second") || slices.Contains(markers, "third") {
		t.Errorf("%s: a method that had not started ran: markers %v", name, markers)
	}
	if !testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll") {
		t.Errorf("%s: markers %v, want context ended, suite.AfterAll, fixture.AfterAll in order", name, markers)
	}
	pid := markerValue(markers, "pid")
	if pid == 0 {
		t.Errorf("%s: the suite published no pid: markers %v", name, markers)
	} else if !processGone(pid) {
		t.Errorf("%s: the suite process %d outlived the run", name, pid)
	}
}

// A suite process whose runner is killed while a method holds stops by
// itself at its channel's end, and ends what its tests started.
func (s *CanaryLifecycleTestSuite) TestASuiteWhoseRunnerIsKilledStops(t *gotest.T) {
	dir := t.TempDir()
	r := s.startRun(t, dir, []string{"STOPPING_CHILD=1"}, "-json", "./testdata/stopping/")
	if !r.await(90*time.Second, "running", "pid=", "child=") {
		_, _ = r.end(0)
		fatalf(t, "runnergone: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	// Killed, the runner's end of the channel closes.
	_ = r.cmd.Process.Kill()
	_, _ = r.end(30 * time.Second)
	awaitMarker(r.markers, selfStopWithin, "fixture.AfterAll")
	markers, _ := testkit.ReadMarkers(r.markers)
	// Before selfStopped, whose check kills the suite's group.
	child := markerValue(markers, "child")
	if child == 0 {
		t.Errorf("runnergone: the test published no child: markers %v", markers)
	} else if !testkit.EndedWithin(child, 3*time.Second) {
		_ = syscall.Kill(child, syscall.SIGKILL)
		t.Errorf("runnergone: the process %d the holding method started outlived the suite", child)
	}
	selfStopped(t, "runnergone", markers)
}
