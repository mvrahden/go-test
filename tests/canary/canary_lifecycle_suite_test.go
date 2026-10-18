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

// A goroutine BeforeAll starts on a T that NewTWithDeadline derives from the
// hook's is still waited for after AfterAll, which is what stops it.
func (s *CanaryTestSuite) TestADerivedTsGoroutineIsWaitedForAfterAfterAll(t *gotest.T) {
	// go test's own alarm bounds a deadlocked wait; a passing run never reaches it.
	_, rows := s.run(t, "setupwaits", t.TempDir(), "-timeout=20s", "-run=TestDerivedTestSuite")
	if !slices.Contains(rows, "setupwaits pass TestDerivedTestSuite") {
		t.Errorf("setupwaits: the suite whose BeforeAll started a goroutine on a derived T did not pass: %v", rows)
	}
}

// When the last of several goroutines BeforeAll started fails or panics, the
// suite fails and every other one is waited for before the fixture goes.
func (s *CanaryTestSuite) TestEveryGoroutineBeforeAllStartedIsWaitedFor(t *gotest.T) {
	dir := t.TempDir()
	_, rows := s.run(t, "setupwaits", dir, "-timeout=20s", "-run=TestFailLastTestSuite|TestPanicLastTestSuite")
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	for _, c := range []struct{ suite, prefix string }{
		{"TestFailLastTestSuite", "fail: "},
		{"TestPanicLastTestSuite", "panic: "},
	} {
		if !slices.Contains(rows, "setupwaits fail "+c.suite) {
			t.Errorf("setupwaits: %s did not fail: %v", c.suite, rows)
		}
		if !testkit.MarkerOrderOf(got, c.prefix+"first goroutine ended", c.prefix+"fixture.AfterAll") {
			t.Errorf("setupwaits: %s released its fixture before every goroutine ended: markers %v", c.suite, got)
		}
	}
}

// outputOf is what the last run's stream printed for the test at path.
func (s *CanaryTestSuite) outputOf(path string) string {
	var out strings.Builder
	for _, ev := range testkit.ParseEvents([]byte(s.lastStream)) {
		if ev.Test == path && ev.Action == "output" {
			out.WriteString(ev.Output)
		}
	}
	return out.String()
}

// A goroutine of an async method other than the one that owns done fails:
// the method fails, and the wait ends only when the owner calls done, fails
// or outlasts the Timeout. AfterEach marks the wait's end.
func (s *CanaryTestSuite) TestAnotherGoroutinesFailureLeavesTheWaitToTheOwner(t *gotest.T) {
	dir := t.TempDir()
	_, rows := s.run(t, "asynchelper", dir, "-run=TestHelperFailsTestSuite")
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	for _, c := range []struct{ method, ends, says string }{
		{"TestOwnerCallsDoneAsync", "owner called done", ""},
		{"TestOwnerFailsAsync", "owner failed", "the owner of done fails"},
		{"TestOwnerHangsAsync", "", "TestOwnerHangsAsync: done() was not called before the test deadline"},
	} {
		path := "TestHelperFailsTestSuite/" + c.method
		if !slices.Contains(rows, "asynchelper fail "+path) {
			t.Errorf("asynchelper: %s did not fail: %v", path, rows)
		}
		out := s.outputOf(path)
		if !strings.Contains(out, "a helper goroutine fails") || !strings.Contains(out, c.says) {
			t.Errorf("asynchelper: %s's output lacks the helper's failure or %q:\n%s", path, c.says, out)
		}
		if !slices.Contains(got, "AfterEach "+path) {
			t.Errorf("asynchelper: %s ran no AfterEach: markers %v", path, got)
		}
		if c.ends != "" && !testkit.MarkerOrderOf(got, c.ends, "AfterEach "+path) {
			t.Errorf("asynchelper: %s: the wait ended before %s: markers %v", path, c.ends, got)
		}
	}
}

// An async method that calls done in time is still held to the Timeout in
// its AfterEach.
func (s *CanaryTestSuite) TestAnAsyncAfterEachIsHeldToTheTimeout(t *gotest.T) {
	_, rows := s.run(t, "asynchelper", t.TempDir(), "-run=TestHookOverrunTestSuite")
	path := "TestHookOverrunTestSuite/TestCallsDoneAsync"
	if !slices.Contains(rows, "asynchelper fail "+path) || !strings.Contains(s.outputOf(path), "exceeded its configured Timeout of 1s") {
		t.Errorf("asynchelper: %s did not fail for its AfterEach's overrun: %v\n%s", path, rows, s.outputOf(path))
	}
}

// liveRun is a CLI run the canary stops or cuts off from outside.
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
	r.cmd.Env = append(childEnv(dir), env...)
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

// await waits up to within for every marker in wants, each matched whole or,
// ending in "=", as a prefix. It reports false when the time is up or the run
// ended first.
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

// mark appends one line to the run's markers file, as the fixtures do.
func (r *liveRun) mark(event string) {
	f, err := os.OpenFile(r.markers, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(event + "\n")
	_ = f.Close()
}

// interrupted runs the stopgrace fixture with env and args, interrupts it once
// a method marked running, and returns how long it took to end after the
// signal, the stream and the markers. The marker "interrupted" is written
// just before the signal; a run that outlives the stop by 40s is killed.
func (s *CanaryTestSuite) interrupted(t *gotest.T, env []string, args ...string) (time.Duration, string, []string) {
	r := s.startRun(t, t.TempDir(), env, append([]string{"-json"}, append(args, "./testdata/stopgrace/")...)...)
	if r.await(90*time.Second, "running") {
		r.mark("interrupted")
		_ = testkit.Interrupt(r.cmd, r.tree)
	}
	began := time.Now()
	_, markers := r.end(40 * time.Second)
	return time.Since(began), r.stdout.String() + r.stderr.String(), markers
}

// A stopped run cuts a body that ignores its context short after 5s, saying
// so, and releases the fixture without running AfterAll.
func (s *CanaryTestSuite) TestABodyIgnoringItsContextIsCutShort(t *gotest.T) {
	took, out, markers := s.interrupted(t, []string{"STOPGRACE_IGNORE=1"}, "-run=TestGraceTestSuite")
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
	_, out, markers := s.interrupted(t, []string{"STOPGRACE_SLOW_AFTEREACH=1"}, "-run=TestGraceTestSuite")
	if !testkit.MarkerOrderOf(markers, "context ended", "AfterEach began", "AfterEach ended", "suite.AfterAll", "fixture.AfterAll") || strings.Contains(out, "cut short") {
		t.Errorf("stopgrace: markers %v, want AfterEach to its end, then AfterAll, then the fixture released\n%s", markers, out)
	}
}

// A stop ends an async wait that a helper's failure left to the owner of
// done, then AfterEach and AfterAll run, and the method fails.
func (s *CanaryTestSuite) TestAStopEndsAnAsyncWaitAfterAFailure(t *gotest.T) {
	_, out, markers := s.interrupted(t, nil, "-run=TestAsyncHoldTestSuite")
	if !testkit.MarkerOrderOf(markers, "interrupted", "async AfterEach", "async AfterAll") {
		t.Errorf("stopgrace: markers %v, want interrupted, async AfterEach, async AfterAll in order", markers)
	}
	verdict := ""
	for _, ev := range testkit.ParseEvents([]byte(out)) {
		if ev.Test == "TestAsyncHoldTestSuite/TestWaitsAsync" && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip") {
			verdict = ev.Action
		}
	}
	if verdict != "fail" {
		t.Errorf("stopgrace: TestWaitsAsync verdict %q, want fail", verdict)
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
while [ "$(cat "$pidfile" 2>/dev/null)" != "$child" ]; do sleep 0.02; done
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
// going 15s after the stop is killed: the holding method's 30s context would
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
	verdict := map[string]string{}
	output := map[string]string{}
	for _, ev := range testkit.ParseEvents(r.stdout.Bytes()) {
		switch ev.Action {
		case "pass", "fail", "skip":
			verdict[ev.Test] = ev.Action
		case "output":
			output[ev.Test] += ev.Output
		}
	}
	for _, method := range []string{"TestStopTestSuite/TestSecond", "TestStopTestSuite/TestThird"} {
		if verdict[method] != "skip" || !strings.Contains(output[method], "run stopping") {
			t.Errorf("stopfile: %s verdict %q, want skip with the reason run stopping:\n%s", method, verdict[method], output[method])
		}
	}
	for _, path := range []string{"TestStopTestSuite/TestHolds", "TestStopTestSuite", ""} {
		if verdict[path] == "" {
			t.Errorf("stopfile: no verdict for %q reached the stream", path)
		}
	}
	if !testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll") {
		t.Errorf("stopfile: markers %v, want context ended, suite.AfterAll, fixture.AfterAll in order", markers)
	}
}
