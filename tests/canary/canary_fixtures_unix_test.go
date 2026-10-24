//go:build unix

package canary_test //nolint:fail-guard

import (
	"os"
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
// Timed from the started process's first write, so the builds before it are
// not counted.
func (s *CanaryTestSuite) TestAProcessTheSetupStartsGetsNoProtocol(t *gotest.T) {
	dir := s.check(t, "detachedchild")
	ended := time.Now()
	if data, err := os.ReadFile(filepath.Join(dir, "detached-pid")); err == nil {
		if pid, err := strconv.Atoi(string(data)); err == nil {
			defer func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }()
		}
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if !slices.Contains(got, "env=") {
		t.Errorf("detachedchild: the started process saw the protocol variable: markers %v", got)
	}
	if strings.Contains(s.lastStream, "written by a process the fixture started") {
		t.Errorf("detachedchild: what the started process wrote to descriptor 3 was read")
	}
	if ready, err := os.Stat(filepath.Join(dir, "detached-ready")); err == nil {
		if took := ended.Sub(ready.ModTime()); took >= 25*time.Second {
			t.Errorf("detachedchild: the run took %s, waiting for the 30s process", took)
		}
	}
}
