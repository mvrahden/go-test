//go:build linux || darwin

package canary_test //nolint:fail-guard

import (
	"errors"
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

// execWrapper writes an -exec wrapper script with body and returns its path.
func execWrapper(t *gotest.T, body string) string {
	path := filepath.Join(t.TempDir(), "wrapper.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil { //nolint:gosec // G306: the script must be executable
		fatalf(t, "write wrapper: %v", err)
	}
	return path
}

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

// A suite process under the runner whose parent is killed while a method
// holds stops by itself.
func (s *CanaryLifecycleTestSuite) TestASuiteWhoseParentIsKilledStops(t *gotest.T) {
	dir := t.TempDir()
	// sh stays the suite's parent: it waits for the command it runs.
	wrap := execWrapper(t, `echo "wrapper=$$" >> "$GOTEST_CANARY_DIR/markers"
"$@"
`)
	r := s.startRun(t, dir, nil, "-json", "-exec", wrap, "./testdata/stopping/")
	if !r.await(90*time.Second, "running", "wrapper=", "pid=") {
		_, _ = r.end(0)
		fatalf(t, "parentgone: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	got, _ := testkit.ReadMarkers(r.markers)
	_ = syscall.Kill(markerValue(got, "wrapper"), syscall.SIGKILL)
	awaitMarker(r.markers, selfStopWithin, "fixture.AfterAll")
	markers, _ := testkit.ReadMarkers(r.markers)
	selfStopped(t, "parentgone", markers)
	_, _ = r.end(30 * time.Second)
}

// A suite process whose runner and converter are killed under -json, while
// an -exec wrapper keeps it running, stops by itself: nobody reads its stream.
func (s *CanaryLifecycleTestSuite) TestASuiteWhoseStreamNobodyReadsStops(t *gotest.T) {
	dir := t.TempDir()
	// The wrapper stays between the converter and the suite, and outlives both.
	wrap := execWrapper(t, `echo "converter=$PPID" >> "$GOTEST_CANARY_DIR/markers"
"$@"
`)
	r := s.startRun(t, dir, nil, "-json", "-exec", wrap, "./testdata/stopping/")
	if !r.await(90*time.Second, "running", "converter=", "pid=") {
		_, _ = r.end(0)
		fatalf(t, "readergone: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	got, _ := testkit.ReadMarkers(r.markers)
	_ = r.cmd.Process.Kill()
	_ = syscall.Kill(markerValue(got, "converter"), syscall.SIGKILL)
	_, _ = r.end(30 * time.Second)
	awaitMarker(r.markers, selfStopWithin, "fixture.AfterAll")
	markers, _ := testkit.ReadMarkers(r.markers)
	selfStopped(t, "readergone", markers)
}

// Outside the runner a suite whose stdout nobody reads any more runs on: its
// method's context does not end.
func (s *CanaryLifecycleTestSuite) TestAPlainRunKeepsItsMethodWhenTheReaderGoes(t *gotest.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	staged := t.TempDir()
	if _, err := testkit.StagePackage(root, filepath.Join("testdata", "plainreader"), staged, "example.com/plainreader"); err != nil {
		fatalf(t, "stage plainreader: %v", err)
	}
	generate := exec.Command(s.binary, "generate", "./plainreader/") //nolint:gosec // G204: controlled binary with fixed args
	generate.Dir = staged
	generate.Env = childEnv(t.TempDir())
	if out, err := generate.CombinedOutput(); err != nil {
		fatalf(t, "generate plainreader: %v\n%s", err, out)
	}
	binary := filepath.Join(staged, "plainreader.test")
	build := exec.Command("go", "test", "-c", "-o", binary, "./plainreader/")
	build.Dir = staged
	if out, err := build.CombinedOutput(); err != nil {
		fatalf(t, "build plainreader: %v\n%s", err, out)
	}
	dir := t.TempDir()
	rd, wr, err := os.Pipe()
	if err != nil {
		fatalf(t, "pipe: %v", err)
	}
	cmd := exec.Command(binary, "-test.run=^TestReaderTestSuite$") //nolint:gosec // G204: binary built by this test
	cmd.Env = childEnv(dir)
	cmd.Stdout = wr
	if err := cmd.Start(); err != nil {
		fatalf(t, "start plainreader: %v", err)
	}
	// Nobody reads the stream from here on.
	_ = wr.Close()
	_ = rd.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	markers, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if !slices.Contains(markers, "not stopped") || slices.Contains(markers, "context ended") {
		t.Errorf("plainreader: the method's context ended when its reader went: markers %v", markers)
	}
}
