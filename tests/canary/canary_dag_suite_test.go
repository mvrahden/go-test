package canary_test //nolint:fail-guard

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// before reports whether markers holds a and, later, b.
func before(markers []string, a, b string) bool {
	i, j := slices.Index(markers, a), slices.Index(markers, b)
	return i >= 0 && j > i
}

// Shared fixtures started together come up each after what it depends on,
// and are released concurrently, each after the fixtures depending on it.
func (s *CanaryTestSuite) TestSharedFixturesStartAndReleaseInDependencyOrder(t *gotest.T) {
	dir := s.check(t, "dagrelease")
	markers, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if !before(markers, "a.BeforeAll ends", "child.BeforeAll begins") {
		t.Errorf("dagrelease: a fixture started before its dependency was up: markers %v", markers)
	}
	if !before(markers, "child.AfterAll ends", "a.AfterAll begins") {
		t.Errorf("dagrelease: a dependent was released after its dependency: markers %v", markers)
	}
	if !before(markers, "b.AfterAll begins", "child.AfterAll ends") || !before(markers, "child.AfterAll begins", "b.AfterAll ends") {
		t.Errorf("dagrelease: independent fixtures were not released together: markers %v", markers)
	}
}

// With a 10s fixture released at the barrier and A (2s), Child (3s, on A) and
// B (2s) resident, a release that never returns is force-killed after 30s
// plus the longest chain still resident: 35s.
func (s *CanaryTestSuite) TestTheTeardownBudgetIsTheLongestResidentChain(t *gotest.T) {
	code, _ := s.runWith(t, "dagrelease", t.TempDir(), []string{"DAGRELEASE_HANG=1"})
	if code != 1 {
		t.Errorf("dagbudget: exit code %d, want 1", code)
	}
	if !strings.Contains(s.lastStderr, "force-killed after 35s;") {
		t.Errorf("dagbudget: the hung release was not force-killed after 35s:\n%s", s.lastStderr)
	}
}
