package canary_test //nolint:fail-guard

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// liveRun is a CLI run the canary stops from outside.
type liveRun struct {
	cmd            *exec.Cmd
	tree           *proctree.Tree
	stdout, stderr bytes.Buffer
	markers        string
	exited         chan struct{}
}

// startRun starts the CLI over args with the marker dir and extra env, as the
// root of its own process tree.
func (s *CanaryTestSuite) startRun(t *gotest.T, dir string, env []string, args ...string) *liveRun {
	r := &liveRun{markers: filepath.Join(dir, testkit.MarkersFile), exited: make(chan struct{})}
	r.cmd = exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	r.cmd.Env = childEnv(dir, env...)
	r.cmd.Stdout, r.cmd.Stderr = &r.stdout, &r.stderr
	r.cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(r.cmd)
	if err != nil {
		fatalf(t, "start: %v", err)
	}
	r.tree = tree
	go func() { _ = r.cmd.Wait(); close(r.exited) }()
	return r
}

// await waits up to within for every marker in wants; a want ending in "="
// matches that key with any value. It reports false when the time is up or
// the run ended first.
func (r *liveRun) await(within time.Duration, wants ...string) bool {
	for deadline := time.Now().Add(within); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(r.markers)
		if !slices.ContainsFunc(wants, func(w string) bool {
			return !slices.ContainsFunc(got, func(m string) bool {
				return m == w || strings.HasSuffix(w, "=") && strings.HasPrefix(m, w)
			})
		}) {
			return true
		}
		select {
		case <-r.exited:
			return false
		default:
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

// end waits up to within for the CLI to exit, kills its tree when it has
// not, and returns what the run left behind. The exit code is -2 for a run
// that had to be killed.
func (r *liveRun) end(within time.Duration) (code int, markers []string) {
	select {
	case <-r.exited:
		code = r.cmd.ProcessState.ExitCode()
	case <-time.After(within):
		_ = r.tree.Kill()
		<-r.exited
		code = -2
	}
	r.tree.Release()
	markers, _ = testkit.ReadMarkers(r.markers)
	return code, markers
}

// interrupted runs pkg with env and args, interrupts it once a method marked
// running, marking "interrupted" just before the signal, and returns how long it took to end after the signal, its exit
// code, its output and the markers. A run that never holds is killed at
// once, one that outlives the stop by 40s then. The time is taken before the
// signal is sent: on Windows the interrupt returns only once its helper
// process has exited, well after the run saw the stop.
func (s *CanaryTestSuite) interrupted(t *gotest.T, pkg string, env []string, args ...string) (took time.Duration, code int, out string, markers []string) {
	return s.interruptedRun(t, pkg, env, append(append([]string{"-json"}, args...), "./testdata/"+pkg+"/"), nil)
}

// interruptedRun is interrupted over the CLI's whole argument list; then,
// when not nil, runs on the live run right after the interrupt.
func (s *CanaryTestSuite) interruptedRun(t *gotest.T, label string, env, args []string, then func(*liveRun)) (took time.Duration, code int, out string, markers []string) {
	r := s.startRun(t, t.TempDir(), env, args...)
	if !r.await(90*time.Second, "running") {
		_, _ = r.end(0)
		fatalf(t, "%s: the run never held:\n%s%s", label, r.stdout.String(), r.stderr.String())
	}
	if f, err := os.OpenFile(r.markers, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		_, _ = f.WriteString("interrupted\n")
		_ = f.Close()
	}
	began := time.Now()
	if err := testkit.Interrupt(r.cmd, r.tree); err != nil {
		_, _ = r.end(0)
		fatalf(t, "%s: signal: %v", label, err)
	}
	if then != nil {
		then(r)
	}
	code, markers = r.end(40 * time.Second)
	return time.Since(began), code, r.stdout.String() + r.stderr.String(), markers
}

// streamVerdict is the last verdict a -json stream carries for the test at
// path: pass, fail, skip, or empty when it carries none.
func streamVerdict(stream, path string) string {
	verdict := ""
	for _, ev := range testkit.ParseEvents([]byte(stream)) {
		if ev.Test == path && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip") {
			verdict = ev.Action
		}
	}
	return verdict
}

// A run stopped while a method holds: the methods not yet started are skipped
// with the reason run stopping, the holding method's context ends, AfterAll
// runs before the fixture is released, and the CLI exits 130. Ring 0: the
// conformance suite that states the same cannot judge its own assertion
// engine.
func (s *CanaryTestSuite) TestAStopSkipsTheMethodsNotStarted(t *gotest.T) {
	_, code, out, markers := s.interrupted(t, "stopping", nil)
	if code != 130 {
		t.Errorf("stop: exit code %d, want 130\n%s", code, out)
	}
	for _, method := range []string{"TestStopTestSuite/TestSecond", "TestStopTestSuite/TestThird"} {
		if v := streamVerdict(out, method); v != "skip" {
			t.Errorf("stop: %s verdict %q, want skip\n%s", method, v, out)
		}
		if !strings.Contains(streamOutput(out, method), "run stopping") {
			t.Errorf("stop: %s was not skipped with the reason run stopping:\n%s", method, out)
		}
	}
	if !slices.Contains(markers, "context ended") {
		t.Errorf("stop: markers %v, want [context ended ...]: the holding method's context did not end", markers)
	}
	if !testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll") {
		t.Errorf("stop: markers %v, want [context ended suite.AfterAll fixture.AfterAll] in order", markers)
	}
}

// A stopped run cuts a body that ignores its context short after 5s, saying
// so, and releases the fixture without running AfterAll.
func (s *CanaryTestSuite) TestABodyIgnoringItsContextIsCutShort(t *gotest.T) {
	took, _, out, markers := s.interrupted(t, "stopgrace", []string{"STOPGRACE_IGNORE=1"}, "-run=TestGraceTestSuite")
	if !strings.Contains(out, "a method did not return within 5s of its context ending; it is cut short") || took < 5*time.Second || took > 20*time.Second {
		t.Errorf("stopgrace: the body was not cut short after 5s, saying so (the run ended %s after the stop):\n%s", took, out)
	}
	if !slices.Contains(markers, "fixture.AfterAll") || slices.Contains(markers, "suite.AfterAll") || slices.Contains(markers, "body returned") {
		t.Errorf("stopgrace: markers %v, want the fixture released without AfterAll and the body cut short", markers)
	}
}

// The 5s grace bounds only the body: an AfterEach that takes longer runs to
// its end, then AfterAll, then the fixture is released.
func (s *CanaryTestSuite) TestASlowAfterEachRunsToItsEndAfterAStop(t *gotest.T) {
	_, _, out, markers := s.interrupted(t, "stopgrace", []string{"STOPGRACE_SLOW_AFTEREACH=1"}, "-run=TestGraceTestSuite")
	if !testkit.MarkerOrderOf(markers, "context ended", "AfterEach began", "AfterEach ended", "suite.AfterAll", "fixture.AfterAll") || strings.Contains(out, "cut short") {
		t.Errorf("stopgrace: markers %v, want AfterEach to its end, then AfterAll, then the fixture released\n%s", markers, out)
	}
}

// A stopped run cuts a BeforeAll that ignores its context short after 5s,
// saying so, and releases the fixture without running AfterAll.
func (s *CanaryTestSuite) TestABeforeAllIgnoringItsContextIsCutShort(t *gotest.T) {
	took, _, out, markers := s.interrupted(t, "stopgrace", nil, "-run=TestSetupHangTestSuite")
	if !strings.Contains(out, "BeforeAll did not return within 5s of its context ending; it is cut short") || took < 5*time.Second || took > 20*time.Second {
		t.Errorf("stopgrace: BeforeAll was not cut short after 5s, saying so (the run ended %s after the stop):\n%s", took, out)
	}
	if !slices.Contains(markers, "fixture.AfterAll") || slices.Contains(markers, "suite.AfterAll") || slices.Contains(markers, "BeforeAll returned") || slices.Contains(markers, "method ran") {
		t.Errorf("stopgrace: markers %v, want the fixture released without AfterAll and BeforeAll cut short", markers)
	}
}

// A body that starts after the stop, its BeforeEach having outlasted it, gets
// the grace from its own start: it is cut short 5s later, saying so.
func (s *CanaryTestSuite) TestABodyStartingAfterTheStopIsCutShort(t *gotest.T) {
	took, _, out, markers := s.interrupted(t, "stopgrace", nil, "-run=TestLateBodyTestSuite")
	if !strings.Contains(out, "a method did not return within 5s of its context ending; it is cut short") || took < 5*time.Second || took > 20*time.Second {
		t.Errorf("stopgrace: the late body was not cut short 5s after it began, saying so (the run ended %s after the stop):\n%s", took, out)
	}
	if !testkit.MarkerOrderOf(markers, "body began", "fixture.AfterAll") || slices.Contains(markers, "body returned") {
		t.Errorf("stopgrace: markers %v, want the late body cut short and the fixture released", markers)
	}
}

// An AfterEach that waits on its context is not ended by the stop: it runs
// to its end, then AfterAll, then the fixture is released.
func (s *CanaryTestSuite) TestAContextAwareAfterEachRunsToItsEndAfterAStop(t *gotest.T) {
	_, _, out, markers := s.interrupted(t, "stopgrace", nil, "-run=TestCleanupTestSuite")
	if !testkit.MarkerOrderOf(markers, "AfterEach ran to its end", "suite.AfterAll", "fixture.AfterAll") || slices.Contains(markers, "AfterEach context ended") {
		t.Errorf("stopgrace: markers %v, want AfterEach to its end under a context the stop left alone, then AfterAll, then the release\n%s", markers, out)
	}
}

// A run still going when only the fixtures' teardown time is left of its
// release bound is forced, and the forced release still gets that time: the
// fixture's slow AfterAll runs to its end before the process ends.
func (s *CanaryTestSuite) TestAForcedStopLeavesTheReleaseItsTime(t *gotest.T) {
	_, _, out, markers := s.interrupted(t, "stopgrace", nil, "-run=TestStuckCleanupTestSuite")
	if !strings.Contains(out, "the run did not stop within its budget") || !testkit.MarkerOrderOf(markers, "release began", "release ended") {
		t.Errorf("stopgrace: markers %v, want the forced release to run to its end\n%s", markers, out)
	}
}

// A stop leaves the running benchmark to finish, skips the benchmarks not
// started with the reason run stopping, then runs AfterAll before the
// fixture is released.
func (s *CanaryTestSuite) TestAStopSkipsTheBenchmarksNotStarted(t *gotest.T) {
	_, _, out, markers := s.interruptedRun(t, "stopbench", nil, []string{"bench", "./testdata/stopbench/", "-args", "-test.benchtime=1x"}, nil)
	if !testkit.MarkerOrderOf(markers, "first returned", "suite.AfterAll", "fixture.AfterAll") || slices.Contains(markers, "second ran") || !strings.Contains(out, "run stopping") {
		t.Errorf("stopbench: markers %v, want the second benchmark skipped as run stopping, then AfterAll, then the release\n%s", markers, out)
	}
}

// A benchmark that ignores the stop is cut short 5s after it, saying so, and
// the fixture is released.
func (s *CanaryTestSuite) TestABenchmarkIgnoringTheStopIsCutShort(t *gotest.T) {
	took, _, out, markers := s.interruptedRun(t, "stopbench", []string{"STOPBENCH_IGNORE=1"}, []string{"bench", "./testdata/stopbench/", "-args", "-test.benchtime=1x"}, nil)
	if !strings.Contains(out, "a benchmark did not return within 5s of its context ending; it is cut short") || took < 5*time.Second || took > 20*time.Second {
		t.Errorf("stopbench: the benchmark was not cut short after 5s, saying so (the run ended %s after the stop):\n%s", took, out)
	}
	if !slices.Contains(markers, "fixture.AfterAll") || slices.Contains(markers, "first returned") {
		t.Errorf("stopbench: markers %v, want the benchmark cut short and the fixture released", markers)
	}
}

// A stop leaves the running fuzz seed to finish, skips the seeds not started
// with the reason run stopping, then runs AfterAll before the fixture is
// released.
func (s *CanaryTestSuite) TestAStopSkipsTheSeedsNotStarted(t *gotest.T) {
	_, _, out, markers := s.interrupted(t, "stopbench", nil)
	if !testkit.MarkerOrderOf(markers, "first seed returned", "suite.AfterAll", "fixture.AfterAll") || slices.Contains(markers, "second seed ran") || !strings.Contains(streamOutput(out, "FuzzSeedStopTestSuite_FuzzSeeds/seed#1"), "run stopping") {
		t.Errorf("stopbench: markers %v, want the second seed skipped as run stopping, then AfterAll, then the release\n%s", markers, out)
	}
}

// A second stop signal while a body ignores the first skips the rest of the
// grace: the fixtures are released at once and the process ends by the
// signal.
func (s *CanaryTestSuite) TestASecondStopSignalReleasesAtOnce(t *gotest.T) {
	if !suiteSignallable {
		t.Skipf("only Unix signals a single process")
	}
	took, _, out, markers := s.interruptedRun(t, "stopgrace", []string{"STOPGRACE_IGNORE=1"}, []string{"-json", "-run=TestGraceTestSuite", "./testdata/stopgrace/"}, func(r *liveRun) {
		time.Sleep(500 * time.Millisecond)
		signalSuite(t, r.markers)
	})
	if !strings.Contains(out, "a second stop signal") || !slices.Contains(markers, "fixture.AfterAll") || took > 4*time.Second {
		t.Errorf("stopgrace: markers %v, want the fixture released at once on the second signal (the run ended %s after the first):\n%s", markers, took, out)
	}
}

// A stop ends an async wait: a method waiting for done is skipped with the
// reason run stopping, one whose helper goroutine failed fails, and AfterEach
// and AfterAll run after the stop.
func (s *CanaryTestSuite) TestAStopEndsAnAsyncWait(t *gotest.T) {
	_, _, out, markers := s.interrupted(t, "stopgrace", nil, "-run=TestAsyncHoldTestSuite")
	waits, fails := "TestAsyncHoldTestSuite/TestWaitsAsync", "TestAsyncHoldTestSuite/TestFailsAsync"
	if v := streamVerdict(out, waits); v != "skip" || !strings.Contains(streamOutput(out, waits), "run stopping") {
		t.Errorf("stopgrace: %s verdict %q, want skip with the reason run stopping\n%s", waits, v, out)
	}
	if v := streamVerdict(out, fails); v != "fail" {
		t.Errorf("stopgrace: %s verdict %q, want fail\n%s", fails, v, out)
	}
	for _, method := range []string{waits, fails} {
		if !testkit.MarkerOrderOf(markers, "interrupted", "async AfterEach "+method, "async AfterAll") {
			t.Errorf("stopgrace: markers %v, want interrupted, %s's AfterEach, async AfterAll in order", markers, method)
		}
	}
}

// stopFileWrapper writes an -exec wrapper that leaves the runner only the
// stop file to reach the suite, as on Windows: it publishes its own pid in
// place of the suite's and ignores the signal sent to it. It marks "pid
// swapped" once it has. Windows itself needs none.
func stopFileWrapper(t *gotest.T) ([]string, []string) {
	if runtime.GOOS == "windows" {
		return nil, []string{"running"}
	}
	path := filepath.Join(t.TempDir(), "wrapper.sh")
	script := `#!/bin/sh
trap '' INT TERM
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
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // G306: the script must be executable
		fatalf(t, "write wrapper: %v", err)
	}
	return []string{"-exec", path}, []string{"running", "pid swapped"}
}

// A run interrupted where only the stop file reaches the suite skips the
// methods not started with the reason run stopping, carries every verdict to
// the stream and runs AfterAll before the fixture is released. A run still
// going 15s after the stop is killed: the holding method's context would
// otherwise end it as if stopped.
func (s *CanaryTestSuite) TestOnlyTheStopFileStopsTheSuite(t *gotest.T) {
	wrap, held := stopFileWrapper(t)
	r := s.startRun(t, t.TempDir(), nil, append(append([]string{"-json"}, wrap...), "./testdata/stopping/")...)
	if !r.await(90*time.Second, held...) {
		_, _ = r.end(0)
		fatalf(t, "stopfile: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	_ = testkit.Interrupt(r.cmd, r.tree)
	_, markers := r.end(15 * time.Second)
	out := r.stdout.String()
	for _, method := range []string{"TestStopTestSuite/TestSecond", "TestStopTestSuite/TestThird"} {
		if v := streamVerdict(out, method); v != "skip" || !strings.Contains(streamOutput(out, method), "run stopping") {
			t.Errorf("stopfile: %s verdict %q, want skip with the reason run stopping:\n%s", method, v, streamOutput(out, method))
		}
	}
	for _, path := range []string{"TestStopTestSuite/TestHolds", "TestStopTestSuite", ""} {
		if streamVerdict(out, path) == "" {
			t.Errorf("stopfile: no verdict for %q reached the stream", path)
		}
	}
	if !testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll") {
		t.Errorf("stopfile: markers %v, want context ended, suite.AfterAll, fixture.AfterAll in order", markers)
	}
}
