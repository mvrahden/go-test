//go:build unix

package canary_test //nolint:fail-guard

import (
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// sharedStopRun starts the CLI over one suite of sharedstop with env and
// returns once its method runs; a run that never gets there is killed first.
func (s *canary) sharedStopRun(t *gotest.T, label, suite string, env ...string) *liveRun {
	r := s.startRun(t, t.TempDir(), env, "-json", "-run=^Test"+suite+"$", "./testdata/sharedstop/")
	if !r.await(90*time.Second, "running") {
		_, _ = r.end(0)
		fatalf(t, "%s: the run never held:\n%s%s", label, r.stdout.String(), r.stderr.String())
	}
	return r
}

// interruptRun sends the CLI a SIGINT and returns when; a run it does not
// reach is killed first.
func interruptRun(t *gotest.T, label string, r *liveRun) time.Time {
	sent := time.Now()
	if err := testkit.Interrupt(r.cmd, r.tree); err != nil {
		_, _ = r.end(0)
		fatalf(t, "%s: signal: %v", label, err)
	}
	return sent
}

// sharedEnded reports whether the shared fixture process marked "pid=<n>"
// has exited within 2s; one still running is killed.
func sharedEnded(markers []string) bool {
	for _, m := range markers {
		if v, ok := strings.CutPrefix(m, "pid="); ok {
			pid, err := strconv.Atoi(v)
			if err != nil || pid <= 0 {
				return false
			}
			if testkit.EndedWithin(pid, 2*time.Second) {
				return true
			}
			_ = syscall.Kill(pid, syscall.SIGKILL)
			return false
		}
	}
	return false
}

// A stopped run asks its shared fixture process to stop by a line on its
// stdin: the fixture's AfterAll runs, the process exits on its own, and no
// signal reaches it.
func (s *CanaryDagTestSuite) TestAStopReachesTheSharedFixtureAsALine(t *gotest.T) {
	r := s.sharedStopRun(t, "sharedstop", "HoldTestSuite")
	interruptRun(t, "sharedstop", r)
	code, markers := r.end(20 * time.Second)
	out := r.stdout.String() + r.stderr.String()
	switch code {
	case 130:
	case -2:
		t.Errorf("sharedstop: the run did not end within 20s of the stop: the shared fixture process was not stopped\n%s", out)
	default:
		t.Errorf("sharedstop: exit code %d, want 130\n%s", code, out)
	}
	if !slices.Contains(markers, "AfterAll ends") {
		t.Errorf("sharedstop: the shared fixture's AfterAll did not run: markers %v", markers)
	}
	if slices.ContainsFunc(markers, func(m string) bool { return strings.HasPrefix(m, "signal ") }) {
		t.Errorf("sharedstop: a signal reached the shared fixture process: markers %v", markers)
	}
	if !sharedEnded(markers) {
		t.Errorf("sharedstop: the shared fixture process outlived the run: markers %v", markers)
	}
}

// A second stop while the shared fixture's AfterAll waits on its context
// ends that context: the AfterAll returns at once, and nothing is killed.
func (s *CanaryDagTestSuite) TestASecondStopEndsTheSharedReleasesContext(t *gotest.T) {
	r := s.sharedStopRun(t, "sharedforce", "HoldTestSuite", "SHAREDSTOP_RELEASE=wait")
	first := interruptRun(t, "sharedforce", r)
	if !r.await(30*time.Second, "AfterAll begins") {
		_, _ = r.end(0)
		fatalf(t, "sharedforce: the shared fixture's AfterAll never began:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	// Past the window in which a repeat is the first delivered again.
	time.Sleep(time.Until(first.Add(time.Second)))
	interruptRun(t, "sharedforce", r)
	code, markers := r.end(10 * time.Second)
	out := r.stdout.String() + r.stderr.String()
	if !slices.Contains(markers, "AfterAll context ended") {
		t.Errorf("sharedforce: the AfterAll's context did not end on the second stop: markers %v", markers)
	}
	if code != 130 {
		t.Errorf("sharedforce: exit code %d, want 130\n%s", code, out)
	}
	if strings.Contains(out, "died during teardown") {
		t.Errorf("sharedforce: the shared fixture process was killed:\n%s", out)
	}
	if !sharedEnded(markers) {
		t.Errorf("sharedforce: the shared fixture process outlived the run: markers %v", markers)
	}
}

// A second stop that reaches the shared fixture process while its suite still
// runs, so before any stop did, stops it too: its AfterAll runs with its
// context ended.
func (s *CanaryDagTestSuite) TestASecondStopFirstStopsTheSharedFixture(t *gotest.T) {
	r := s.sharedStopRun(t, "forcefirst", "IgnoreTestSuite", "SHAREDSTOP_RELEASE=wait")
	first := interruptRun(t, "forcefirst", r)
	time.Sleep(time.Until(first.Add(time.Second)))
	interruptRun(t, "forcefirst", r)
	code, markers := r.end(10 * time.Second)
	out := r.stdout.String() + r.stderr.String()
	switch {
	case !slices.Contains(markers, "AfterAll begins"):
		t.Errorf("forcefirst: the second stop did not stop the shared fixture process: markers %v", markers)
	case !slices.Contains(markers, "AfterAll context ended"):
		t.Errorf("forcefirst: the AfterAll's context did not end on the second stop: markers %v", markers)
	}
	if code != 130 {
		t.Errorf("forcefirst: exit code %d, want 130\n%s", code, out)
	}
	if !sharedEnded(markers) {
		t.Errorf("forcefirst: the shared fixture process outlived the run: markers %v", markers)
	}
}
