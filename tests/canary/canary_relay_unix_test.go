//go:build unix

package canary_test //nolint:fail-guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// relayWrappers writes the -exec wrappers the relay checks run suites
// under into dir.
func relayWrappers(t *gotest.T, dir string) {
	for name, script := range map[string]string{
		// macOS's /bin/sh, bash 3.2, keeps a copy of a descriptor it closes on
		// an exec that also runs a command, and the command inherits it: each
		// wrapper below closes descriptor 3 with an exec of its own first.
		// Runs the suite with the channel's descriptor closed: EOF at once.
		"close3.sh": "#!/bin/sh\nexec 3>&-\nexec \"$@\"\n",
		// Keeps the channel open and silent, the suite without it: no hello
		// within the 3s the runner waits.
		"hold3.sh": "#!/bin/sh\n\"$@\" 3>&-\n",
		// Says hello for the suite, then runs it with the channel closed at
		// its end: the stop line cannot be written.
		"hello3.sh": "#!/bin/sh\nprintf 'hello 1 %d\\n' $$ >&3\nexec 3>&-\nexec \"$@\"\n",
		// Runs the suite in the background and exits at once.
		"fork.sh": "#!/bin/sh\n\"$@\" &\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil { //nolint:gosec // G306: the wrapper must be executable
			fatalf(t, "write %s: %v", name, err)
		}
	}
}

// relayHeld starts the CLI over one suite of the relay fixture with args in
// front, and returns once its first method runs; a run that never gets there
// is killed first.
func (s *canary) relayHeld(t *gotest.T, label, suite string, env []string, args ...string) *liveRun {
	args = append(append([]string{"-json"}, args...), "-run=^Test"+suite+"$", "./testdata/relay/")
	r := s.startRun(t, t.TempDir(), env, args...)
	if !r.await(90*time.Second, "running") {
		_, _ = r.end(0)
		fatalf(t, "%s: the run never held:\n%s%s", label, r.stdout.String(), r.stderr.String())
	}
	return r
}

// ctrlC sends the CLI's process group a SIGINT, as a terminal's Ctrl-C does;
// a run it does not reach is killed first.
func ctrlC(t *gotest.T, label string, r *liveRun) {
	if err := syscall.Kill(-r.cmd.Process.Pid, syscall.SIGINT); err != nil {
		_, _ = r.end(0)
		fatalf(t, "%s: signal: %v", label, err)
	}
}

// relayStopped checks a run of HoldTestSuite that a Ctrl-C stopped while its
// suite could not take the stop line: the suite's group got one SIGINT, the
// converter none, every method has its verdict and the suite tore down.
func relayStopped(t *gotest.T, label string, code int, out string, markers []string) {
	switch code {
	case 130:
	case -2:
		t.Errorf("%s: the run did not end within 15s of the stop: no Ctrl-C reached the suite\n%s", label, out)
	default:
		t.Errorf("%s: exit code %d, want 130\n%s", label, code, out)
	}
	if !slices.Contains(markers, "suite interrupt") || !slices.Contains(markers, "child SIGINT") {
		t.Errorf("%s: the suite's process group got no SIGINT: markers %v", label, markers)
	}
	if strings.Contains(out, "the converter ended early") {
		t.Errorf("%s: the stop reached the converter:\n%s", label, out)
	}
	if v := streamVerdict(out, "TestHoldTestSuite/TestHolds"); v != "pass" {
		t.Errorf("%s: TestHolds verdict %q, want pass\n%s", label, v, out)
	}
	for _, method := range []string{"TestHoldTestSuite/TestSecond", "TestHoldTestSuite/TestThird"} {
		if v := streamVerdict(out, method); v != "skip" || !strings.Contains(streamOutput(out, method), "run stopping") {
			t.Errorf("%s: %s verdict %q, want skip with the reason run stopping\n%s", label, method, v, out)
		}
	}
	if !testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll") {
		t.Errorf("%s: markers %v, want context ended, suite.AfterAll, fixture.AfterAll in order", label, markers)
	}
}

// relayCtrlC runs HoldTestSuite under the -exec wrapper, stops it with a
// Ctrl-C and checks the run with relayStopped.
func (s *canary) relayCtrlC(t *gotest.T, wrapper string) {
	wrappers := t.TempDir()
	relayWrappers(t, wrappers)
	label := "relay " + wrapper
	r := s.relayHeld(t, label, "HoldTestSuite", nil, "-exec", filepath.Join(wrappers, wrapper))
	ctrlC(t, label, r)
	code, markers := r.end(15 * time.Second)
	relayStopped(t, label, code, r.stdout.String()+r.stderr.String(), markers)
}

// A suite that never says hello is stopped by one SIGINT to its process
// group, which the converter beside it never gets: under a wrapper that runs
// it with descriptor 3 closed, at the channel's end, and under one that
// holds the channel silent, 3s after its start.
func (s *CanaryRelayTestSuite) TestASuiteWithoutItsChannelGetsACtrlC(t *gotest.T) {
	s.relayCtrlC(t, "close3.sh")
	s.relayCtrlC(t, "hold3.sh")
}

// A suite whose stop line cannot be written gets the same Ctrl-C.
func (s *CanaryRelayTestSuite) TestAStopLineThatCannotBeWrittenIsACtrlC(t *gotest.T) {
	s.relayCtrlC(t, "hello3.sh")
}

// converterUnder finds the converter among the descendants of pid: the
// process whose command is test2json, 0 when there is none.
func converterUnder(pid int) int {
	for queue := []int{pid}; len(queue) > 0; queue = queue[1:] {
		out, _ := exec.Command("pgrep", "-P", strconv.Itoa(queue[0])).Output() //nolint:gosec // G204: a pid of this check's own run
		for _, field := range strings.Fields(string(out)) {
			child, err := strconv.Atoi(field)
			if err != nil {
				continue
			}
			comm, _ := exec.Command("ps", "-o", "comm=", "-p", field).Output() //nolint:gosec // G204: a pid of this check's own run
			if filepath.Base(strings.TrimSpace(string(comm))) == "test2json" {
				return child
			}
			queue = append(queue, child)
		}
	}
	return 0
}

// A converter killed while the suite runs is replaced by a conversion of
// everything the suite wrote: every verdict is in the stream, and a note
// on stderr names the suite and how the converter ended.
func (s *CanaryRelayTestSuite) TestAKilledConverterIsReplaced(t *gotest.T) {
	r := s.relayHeld(t, "converter", "WaitTestSuite", nil)
	converter := converterUnder(r.cmd.Process.Pid)
	if converter == 0 {
		_, _ = r.end(0)
		fatalf(t, "converter: no converter under the CLI")
	}
	_ = syscall.Kill(converter, syscall.SIGKILL)
	for deadline := time.Now().Add(5 * time.Second); syscall.Kill(converter, 0) == nil && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(r.markers), "release"), nil, 0o600); err != nil {
		_, _ = r.end(0)
		fatalf(t, "converter: release: %v", err)
	}
	code, _ := r.end(40 * time.Second)
	out := r.stdout.String() + r.stderr.String()
	if code != 0 {
		t.Errorf("converter: exit code %d, want 0\n%s", code, out)
	}
	for _, path := range []string{"TestWaitTestSuite/TestWaits", "TestWaitTestSuite/TestAfter/passes_once_released", "TestWaitTestSuite/TestRows/second", "TestWaitTestSuite"} {
		if v := streamVerdict(r.stdout.String(), path); v != "pass" {
			t.Errorf("converter: %s verdict %q, want pass: the stream lost what the suite wrote after the converter died\n%s", path, v, out)
		}
	}
	if !strings.Contains(r.stderr.String(), "TestWaitTestSuite: the converter ended early (signal: killed); the suite's output was converted again") {
		t.Errorf("converter: stderr does not say the output was converted again:\n%s", r.stderr.String())
	}
}

// A signal from outside the runner stops the suite and fails its package,
// saying so.
func (s *CanaryRelayTestSuite) TestASignalFromOutsideFailsThePackage(t *gotest.T) {
	r := s.relayHeld(t, "outside", "HoldTestSuite", nil)
	markers, _ := testkit.ReadMarkers(r.markers)
	pid := 0
	for _, m := range markers {
		if v, ok := strings.CutPrefix(m, "pid="); ok {
			pid, _ = strconv.Atoi(v)
		}
	}
	if pid <= 0 {
		_, _ = r.end(0)
		fatalf(t, "outside: no suite pid in the markers %v", markers)
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	code, markers := r.end(20 * time.Second)
	out := r.stdout.String() + r.stderr.String()
	if code != 1 {
		t.Errorf("outside: exit code %d, want 1\n%s", code, out)
	}
	if v := relayPackage(r.stdout.String()); v != "fail" {
		t.Errorf("outside: package verdict %q, want fail\n%s", v, out)
	}
	if !strings.Contains(r.stderr.String(), "gotest: suite stopped by SIGTERM from outside the runner") {
		t.Errorf("outside: stderr does not name the stop from outside the runner:\n%s", out)
	}
	if !slices.Contains(markers, "fixture.AfterAll") {
		t.Errorf("outside: the fixture was not released: markers %v", markers)
	}
}

// A suite an -exec wrapper started in the background runs to its end after
// the wrapper exited: its channel stays open until its tree is released.
func (s *CanaryRelayTestSuite) TestASuiteOutlivingItsWrapperRunsOn(t *gotest.T) {
	wrappers := t.TempDir()
	relayWrappers(t, wrappers)
	r := s.startRun(t, t.TempDir(), nil, "-json", "-exec", filepath.Join(wrappers, "fork.sh"), "-run=^TestPauseTestSuite$", "./testdata/relay/")
	code, markers := r.end(30 * time.Second)
	out := r.stdout.String() + r.stderr.String()
	if code != 0 {
		t.Errorf("forked: exit code %d, want 0\n%s", code, out)
	}
	if !slices.Contains(markers, "held") || !slices.Contains(markers, "second") || streamVerdict(r.stdout.String(), "TestPauseTestSuite/TestSecond") != "pass" {
		t.Errorf("forked: the suite stopped when its wrapper exited: markers %v\n%s", markers, out)
	}
}
