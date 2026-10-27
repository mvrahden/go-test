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

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A process a test started and never waited for does not outlive the run:
// the suite's whole process group ends with it.
func (s *CanaryLifecycleTestSuite) TestNothingOutlivesTheRun(t *gotest.T) {
	dir := t.TempDir()
	r := s.run(t, "orphan", dir)
	if !slices.Contains(r.rows, "orphan pass TestOrphanTestSuite/TestLeavesAProcess") {
		t.Errorf("orphan: the test did not pass: %v", r.rows)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "orphan-pid"))
	if err != nil {
		t.Errorf("orphan: the test recorded no pid: %v", err)
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Errorf("orphan: the recorded pid %q is no number", raw)
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for sleeping(pid) {
		if time.Now().After(deadline) {
			t.Errorf("orphan: sleep 4242 outlived the run (pid %d)", pid)
			_ = syscall.Kill(pid, syscall.SIGKILL)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// sleeping reports whether pid is still the orphan suite's sleep 4242, not
// a zombie and not a process that reused the pid.
func sleeping(pid int) bool {
	out, _ := exec.Command("ps", "-o", "stat=,args=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // G204: a fixed command, a numeric pid
	stat, args, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	return stat != "" && !strings.HasPrefix(stat, "Z") && strings.TrimSpace(args) == "sleep 4242"
}
