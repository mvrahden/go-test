package fixtures_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureSelectionTestSuite: a shared fixture starts for the suites the run
// dispatches, and for no other.
type FixtureSelectionTestSuite struct{}

func (s *FixtureSelectionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FixtureSelectionTestSuite) TestSkipFlag(t *gotest.T) {
	t.When("-skip names the only suite binding a shared fixture", func(w *gotest.T) {
		markerDir := w.TempDir()
		run, err := testkit.RunPackage(w.Context(), markerDir, "../testdata/skipped/", "-skip=TestBoundTestSuite")
		gotest.NoError(w, err)
		markers, err := testkit.ReadMarkers(filepath.Join(markerDir, "markers"))
		gotest.NoError(w, err)
		w.It("the fixture is never started and the other suite passes", func(it *gotest.T) {
			gotest.NotContains(it, markers, "shared up")
			gotest.Equal(it, "pass", run.Verdict("TestFreeTestSuite/TestOne"), "stdout: %s", run.Stdout)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
		})
	})
	t.When("-skip names one method of the suite binding a shared fixture", func(w *gotest.T) {
		markerDir := w.TempDir()
		run, err := testkit.RunPackage(w.Context(), markerDir, "../testdata/skipped/", "-skip=TestBoundTestSuite/TestOne")
		gotest.NoError(w, err)
		markers, err := testkit.ReadMarkers(filepath.Join(markerDir, "markers"))
		gotest.NoError(w, err)
		w.It("the fixture starts and the suite's other method passes against it", func(it *gotest.T) {
			gotest.Contains(it, markers, "shared up")
			gotest.Equal(it, "pass", run.Verdict("TestBoundTestSuite/TestTwo"), "stdout: %s", run.Stdout)
			gotest.NotEqual(it, "pass", run.Verdict("TestBoundTestSuite/TestOne"))
		})
	})
}

func (s *FixtureSelectionTestSuite) TestGuardedSuite(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/guardedshared/")
	gotest.NoError(t, err)
	t.When("a guard-skipped suite binds a shared fixture whose setup fails", func(w *gotest.T) {
		w.It("the fixture is started for it and its failure fails the run and the suite, whose guard never runs", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "fail", run.Verdict("TestGuardedTestSuite"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "TestGuardedTestSuite never ran: shared fixture TrapSharedFixture did not come up")
			gotest.Equal(it, "pass", run.Verdict("TestFreeTestSuite/TestOne"))
		})
	})
}
