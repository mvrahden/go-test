//go:build unix

package canary_test //nolint:fail-guard

import (
	"bytes"
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

// A process a shared fixture's setup starts gets neither the protocol
// descriptor nor the variable naming it, and the run does not wait for it.
func (s *CanaryTestSuite) TestAProcessTheSetupStartsGetsNoProtocol(t *gotest.T) {
	dir := t.TempDir()
	began := time.Now()
	r := s.checkFixture(t, "detachedchild", dir)
	took := time.Since(began)
	if data, err := os.ReadFile(filepath.Join(dir, "detached-pid")); err == nil {
		if pid, err := strconv.Atoi(string(data)); err == nil {
			defer func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }()
		}
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if !slices.Contains(got, "env=") {
		t.Errorf("detachedchild: the started process saw the protocol variable: markers %v", got)
	}
	if strings.Contains(r.stdout+r.stderr, "written by a process the fixture started") {
		t.Errorf("detachedchild: what the started process wrote to descriptor 3 was read")
	}
	if took >= 25*time.Second {
		t.Errorf("detachedchild: the run took %s, waiting for the 30s process", took)
	}
}

// The CLI killed while a shared fixture is up: the fixture process releases
// it, its AfterAll running to its end, then exits.
func (s *CanaryTestSuite) TestAKilledCLIsFixtureStillReleases(t *gotest.T) {
	dir := t.TempDir()
	cmd := exec.Command(s.binary, "-json", "./testdata/killedcli/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(dir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	tree, err := testkit.StartCLI(cmd)
	if err != nil {
		fatalf(t, "start: %v", err)
	}
	markers := filepath.Join(dir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, "suite holds") {
			break
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			fatalf(t, "killedcli: the suite never held:\n%s", out.String())
		}
	}
	// The CLI alone: the fixture process lives in a process group of its own.
	if err := cmd.Process.Kill(); err != nil {
		fatalf(t, "killedcli: kill: %v", err)
	}
	_ = cmd.Wait()
	tree.Release()
	pid := 0
	got, _ := testkit.ReadMarkers(markers)
	for _, m := range got {
		if v, ok := strings.CutPrefix(m, "pid="); ok {
			pid, _ = strconv.Atoi(v)
		}
	}
	if pid <= 0 {
		fatalf(t, "killedcli: the fixture marked no pid: %v", got)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	deadline := time.Now().Add(30 * time.Second)
	for syscall.Kill(pid, 0) != syscall.ESRCH {
		if time.Now().After(deadline) {
			t.Errorf("killedcli: the shared fixture process outlived the CLI by 30s")
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	got, _ = testkit.ReadMarkers(markers)
	if !testkit.MarkerOrderOf(got, "AfterAll begins", "AfterAll ends") {
		t.Errorf("killedcli: the fixture's AfterAll did not run to its end: markers %v", got)
	}
}
