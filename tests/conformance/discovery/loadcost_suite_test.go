package discovery_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoveryLoadCostTestSuite: reading a package's suites compiles none of
// its test packages, so a test-file edit leaves the build cache as it was.
type DiscoveryLoadCostTestSuite struct{}

func (s *DiscoveryLoadCostTestSuite) SuiteConfig() gotest.SuiteConfig {
	// A private build cache compiles the dependencies' export data once:
	// about 6s a method, 12s for both, on a 6-core Linux machine.
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

// afterATestFileChanges runs args in a staged copy of the passing fixture
// with a private build cache, appends to its test file, runs args again and
// returns the cache's file count before and after the edit.
func afterATestFileChanges(t *gotest.T, exits []int, args ...string) (before, after int) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir, err := testkit.StagePackage(root, "../testdata/passing", t.TempDir(), "loadcost.example")
	gotest.NoError(t, err)
	cache := t.TempDir()
	run := func() {
		r, err := testkit.RunCLIWithEnv(t.Context(), dir, []string{"GOCACHE=" + cache}, args...)
		gotest.NoError(t, err)
		gotest.Contains(t, exits, r.Exit, "stdout: %s\nstderr: %s", r.Stdout, r.Stderr)
	}
	run() // fills the cache with the dependencies' export data
	before = countFiles(t, cache)
	f, err := os.OpenFile(filepath.Join(dir, "passing_suite_test.go"), os.O_APPEND|os.O_WRONLY, 0)
	gotest.NoError(t, err)
	_, err = f.WriteString("\nvar loadCostMarker = 1\n")
	gotest.NoError(t, err)
	gotest.NoError(t, f.Close())
	run()
	return before, countFiles(t, cache)
}

// countFiles counts the regular files under dir.
func countFiles(t *gotest.T, dir string) int {
	n := 0
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			n++
		}
		return err
	})
	gotest.NoError(t, err)
	return n
}

func (s *DiscoveryLoadCostTestSuite) TestATestFileChanges(t *gotest.T) {
	before, after := afterATestFileChanges(t, []int{0}, "discover", ".")
	t.When("a test file of a discovered package is changed and the package is discovered again", func(w *gotest.T) {
		w.It("the build cache holds no more files than before", func(it *gotest.T) {
			gotest.Equal(it, before, after)
		})
	})
}

func (s *DiscoveryLoadCostTestSuite) TestLintAfterATestFileChanges(t *gotest.T) {
	before, after := afterATestFileChanges(t, []int{0, 3}, "lint", ".")
	t.When("a test file of a linted package is changed and the package is linted again", func(w *gotest.T) {
		w.It("the build cache holds no more files than before", func(it *gotest.T) {
			gotest.Equal(it, before, after)
		})
	})
}
