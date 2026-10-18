//go:build unix

package canary_test //nolint:fail-guard

import (
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A process a test started and never waited for does not outlive the run:
// the suite's whole process group ends with it.
func (s *CanaryTestSuite) TestNothingOutlivesTheRun(t *gotest.T) {
	_, rows := s.run(t, "orphan", t.TempDir())
	if !slices.Contains(rows, "orphan pass TestOrphanTestSuite/TestLeavesAProcess") {
		t.Errorf("orphan: the test did not pass: %v", rows)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, _ := exec.Command("pgrep", "-f", "^sleep 4242$").Output()
		if strings.TrimSpace(string(out)) == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("orphan: sleep 4242 outlived the run (pids %s)", strings.TrimSpace(string(out)))
			_ = exec.Command("pkill", "-f", "^sleep 4242$").Run()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
