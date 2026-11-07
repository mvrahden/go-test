//go:build unix

package canary_test //nolint:fail-guard

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// suiteSignallable: a single suite process can be signalled here.
const suiteSignallable = true

// signalSuite sends SIGTERM to the suite process whose pid its fixture marked
// as "pid=<n>", as a second stop signal reaches it.
func signalSuite(t *gotest.T, markersPath string) {
	got, _ := testkit.ReadMarkers(markersPath)
	for _, m := range got {
		if pid, ok := strings.CutPrefix(m, "pid="); ok {
			n, err := strconv.Atoi(pid)
			if err != nil {
				fatalf(t, "pid marker %q: %v", m, err)
			}
			if err := syscall.Kill(n, syscall.SIGTERM); err != nil {
				t.Errorf("signal the suite: %v", err)
			}
			return
		}
	}
	t.Errorf("no pid marker among %v", got)
}

// A signal that reaches the suite only after the runner's stop settled the
// run leaves it to end with its tests' own status: every verdict reaches the
// stream and the process lives to its own end.
func (s *CanaryLifecycleTestSuite) TestASignalAfterTheStopSettledTheRun(t *gotest.T) {
	// The runner stops the suite by its stop line; the signal comes after.
	r := s.startRun(t, t.TempDir(), []string{"STOPPING_LINGER=1"}, "-json", "./testdata/stopping/")
	if !r.await(90*time.Second, "running") {
		_, _ = r.end(0)
		fatalf(t, "latesignal: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	if err := testkit.Interrupt(r.cmd, r.tree); err != nil {
		_, _ = r.end(0)
		fatalf(t, "latesignal: signal: %v", err)
	}
	if !r.await(30*time.Second, "teardown done") {
		_, _ = r.end(0)
		fatalf(t, "latesignal: the run never settled:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	signalSuite(t, r.markers)
	_, markers := r.end(15 * time.Second)
	out := r.stdout.String()
	if !slices.Contains(markers, "lingered") {
		t.Errorf("latesignal: markers %v, want lingered: the signal ended the process:\n%s", markers, r.stderr.String())
	}
	for _, path := range []string{"TestStopTestSuite/TestHolds", "TestStopTestSuite", ""} {
		if streamVerdict(out, path) == "" {
			t.Errorf("latesignal: no verdict for %q reached the stream:\n%s", path, out)
		}
	}
}

// The runner's first stop reaches the suite as a line on its channel, never
// as a signal: the process ends with its tests' own status, the run's stderr
// names no signal, and the -json stream carries a verdict for every test.
func (s *CanaryLifecycleTestSuite) TestAStopReachesTheSuiteAsALine(t *gotest.T) {
	took, code, out, markers := s.interrupted(t, "stopping", nil)
	if code == -2 || took > 10*time.Second {
		t.Errorf("stopline: the run did not end within 10s of the stop (it took %s):\n%s", took, out)
	}
	if code != 130 {
		t.Errorf("stopline: exit code %d, want 130 (the run ended %s after the stop)\n%s", code, took, out)
	}
	if strings.Contains(out, "terminated by signal") || strings.Contains(out, "from outside the runner") {
		t.Errorf("stopline: a signal reached the suite:\n%s", out)
	}
	if !slices.Contains(markers, "context ended") {
		t.Errorf("stopline: markers %v, want context ended: the stop never reached the suite", markers)
	}
	for _, path := range []string{"TestStopTestSuite", "TestStopTestSuite/TestHolds", "TestStopTestSuite/TestSecond", "TestStopTestSuite/TestThird"} {
		if streamVerdict(out, path) == "" {
			t.Errorf("stopline: no verdict for %q reached the stream:\n%s", path, out)
		}
	}
}

// A stop the runner sends while the suite processes run their TestMain is
// read when their first suite opens: every suite skips with the reason run
// stopping and no fixture is set up.
func (s *CanaryLifecycleTestSuite) TestAStopDuringTestMainSkipsEverySuite(t *gotest.T) {
	r := s.startRun(t, t.TempDir(), nil, "-json", "./testdata/stopatopen/")
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(r.markers)
		if countMarker(got, "main") == 2 {
			break
		}
		if time.Now().After(deadline) {
			_, _ = r.end(0)
			fatalf(t, "stopatopen: the suite processes never ran their TestMain:\n%s%s", r.stdout.String(), r.stderr.String())
		}
	}
	if err := testkit.Interrupt(r.cmd, r.tree); err != nil {
		_, _ = r.end(0)
		fatalf(t, "stopatopen: signal: %v", err)
	}
	// The stop is on its way to the suites; then their TestMain goes on.
	time.Sleep(time.Second)
	if f, err := os.OpenFile(r.markers, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		_, _ = f.WriteString("go\n")
		_ = f.Close()
	}
	code, markers := r.end(40 * time.Second)
	out := r.stdout.String()
	if code != 130 {
		t.Errorf("stopatopen: exit code %d, want 130\n%s%s", code, out, r.stderr.String())
	}
	for _, suite := range []string{"TestFirstTestSuite", "TestSecondTestSuite"} {
		if v := streamVerdict(out, suite); v != "skip" || !strings.Contains(streamOutput(out, suite), "run stopping") {
			t.Errorf("stopatopen: %s verdict %q, want skip with the reason run stopping:\n%s", suite, v, out)
		}
	}
	for _, m := range []string{"fixture.BeforeAll", "first.BeforeAll", "second.BeforeAll"} {
		if slices.Contains(markers, m) {
			t.Errorf("stopatopen: markers %v: the stopped run set a suite up", markers)
			break
		}
	}
}

// countMarker counts the markers equal to want.
func countMarker(markers []string, want string) int {
	n := 0
	for _, m := range markers {
		if m == want {
			n++
		}
	}
	return n
}

// A SIGTERM the runner did not send is a first stop: the holding method's
// context ends and AfterAll runs before the release, the stream names the
// stop as one from outside the runner and the run fails. A second SIGTERM
// during that stop forces nothing.
func (s *CanaryLifecycleTestSuite) TestASignalFromOutsideIsAStop(t *gotest.T) {
	r := s.startRun(t, t.TempDir(), nil, "-json", "./testdata/stopping/")
	if !r.await(90*time.Second, "running", "pid=") {
		_, _ = r.end(0)
		fatalf(t, "outside: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	signalSuite(t, r.markers)
	if !r.await(10*time.Second, "context ended") {
		_, markers := r.end(0)
		fatalf(t, "outside: markers %v: the holding method's context never ended on the signal", markers)
	}
	// The holding method returns 500ms after its context ended; the second
	// signal lands while the stop waits for it.
	signalSuite(t, r.markers)
	code, markers := r.end(30 * time.Second)
	out := r.stdout.String() + r.stderr.String()
	if !testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll") || strings.Contains(out, "torn down now") {
		t.Errorf("outside: markers %v, want AfterAll before the release: the second signal forced the stop\n%s", markers, out)
	}
	if !strings.Contains(streamOutput(r.stdout.String(), ""), "gotest: suite stopped by SIGTERM from outside the runner") {
		t.Errorf("outside: the stream names no stop from outside the runner:\n%s", out)
	}
	if code != 1 || streamVerdict(r.stdout.String(), "") != "fail" {
		t.Errorf("outside: exit code %d, package %q, want 1 and fail\n%s", code, streamVerdict(r.stdout.String(), ""), out)
	}
}

// The runner's channel stays with the suite process: a process a test
// starts inherits none of it, a descriptor named as the channel that is a
// regular file is left open, and no reader of it outlives the last suite.
func (s *CanaryLifecycleTestSuite) TestTheChannelStaysWithTheSuite(t *gotest.T) {
	r := s.startRun(t, t.TempDir(), nil, "-json", "./testdata/channel/")
	code, markers := r.end(90 * time.Second)
	if code != 0 {
		t.Errorf("channel: exit code %d, want 0\n%s%s", code, r.stdout.String(), r.stderr.String())
	}
	// One socket fewer than the suite's own: the channel; the host may hand
	// every process other sockets.
	if own, got := markerNumber(markers, "suite sockets="), markerNumber(markers, "sockets="); own < 1 || got != own-1 {
		t.Errorf("channel: markers %v, want one socket fewer than the suite's: a process the test started inherited the channel", markers)
	}
	if !slices.Contains(markers, "descriptor kept") {
		t.Errorf("channel: markers %v, want descriptor kept: a regular file named as the channel was closed", markers)
	}
	if !slices.Contains(markers, "no reader left") {
		t.Errorf("channel: markers %v, want no reader left: a reader of the channel outlived the last suite", markers)
	}
}

// markerNumber reads the number a marker with prefix carries, -1 without one.
func markerNumber(markers []string, prefix string) int {
	for _, m := range markers {
		if strings.HasPrefix(m, prefix) {
			if n, err := strconv.Atoi(strings.TrimPrefix(m, prefix)); err == nil {
				return n
			}
		}
	}
	return -1
}
