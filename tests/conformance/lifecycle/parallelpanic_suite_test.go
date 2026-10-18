package lifecycle_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelPanicTestSuite: what a panic in one parallel method leaves to its
// siblings, as under go test.
type ParallelPanicTestSuite struct{}

func (s *ParallelPanicTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ParallelPanicTestSuite) TestSiblings(t *gotest.T) {
	markerDir := t.TempDir()
	run, err := testkit.RunPackage(t.Context(), markerDir, "../testdata/parallelpanic/")
	gotest.NoError(t, err)
	markers, err := testkit.ReadMarkers(filepath.Join(markerDir, "markers"))
	gotest.NoError(t, err)
	t.When("a parallel method panics while a sibling is still running", func(w *gotest.T) {
		w.It("AfterAll and the fixture teardown run, the sibling is cut short and its AfterEach never runs, and the run fails", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "fail", run.Verdict("TestParallelPanicTestSuite/TestPanics"), "stdout: %s", run.Stdout)
			gotest.Contains(it, markers, "slow started")
			gotest.Contains(it, markers, "afterall")
			gotest.Contains(it, markers, "fixture down")
			gotest.NotContains(it, markers, "slow finished")
			gotest.NotContains(it, markers, "aftereach TestParallelPanicTestSuite/TestSlow")
		})
	})
}
