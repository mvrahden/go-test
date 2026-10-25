package fixtures_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureSelectionTestSuite: a shared fixture starts for the suites the run
// dispatches, and for no other.
type FixtureSelectionTestSuite struct{}

func (s *FixtureSelectionTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *FixtureSelectionTestSuite) TestSkipFlag(t *gotest.T) {
	t.When("-skip names the only suite binding a shared fixture", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/skipped/", "-skip=TestBoundTestSuite")
		gotest.NoError(w, err)
		w.It("the fixture is never started and the other suite passes", func(it *gotest.T) {
			gotest.NotContains(it, run.Markers, "shared up")
			gotest.Equal(it, "pass", run.Verdict("TestFreeTestSuite/TestOne"), "stdout: %s", run.Stdout)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
		})
	})
	t.When("-skip names one method of the suite binding a shared fixture", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/skipped/", "-skip=TestBoundTestSuite/TestOne")
		gotest.NoError(w, err)
		w.It("the fixture starts and the suite's other method passes against it", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "shared up")
			gotest.Equal(it, "pass", run.Verdict("TestBoundTestSuite/TestTwo"), "stdout: %s", run.Stdout)
			gotest.NotEqual(it, "pass", run.Verdict("TestBoundTestSuite/TestOne"))
		})
	})
}

func (s *FixtureSelectionTestSuite) TestRepeatedSkipFlag(t *gotest.T) {
	t.When("-skip is given twice, naming the fixture-free suite last", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/skipped/", "-skip=TestBoundTestSuite", "-skip=TestFreeTestSuite")
		gotest.NoError(w, err)
		w.It("the last one counts: the fixture starts, its suite passes and the other suite does not run", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "shared up")
			gotest.Equal(it, "pass", run.Verdict("TestBoundTestSuite/TestOne"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Empty(it, run.Verdict("TestFreeTestSuite/TestOne"))
		})
	})
}

func (s *FixtureSelectionTestSuite) TestAlternativeOrder(t *gotest.T) {
	t.When("-skip's first alternative matching a suite names one of its methods and a later one names the suite", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/skipped/", "-skip=TestBoundTestSuite/TestOne|TestBoundTestSuite")
		gotest.NoError(w, err)
		w.It("go test still enters the suite, so its fixture starts and the suite passes", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "shared up")
			gotest.Equal(it, "pass", run.Verdict("TestBoundTestSuite"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
	t.When("-run's first alternative names a method of one suite and a later one names the other suite", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/skipped/", "-run=TestFreeTestSuite/TestOne|TestBoundTestSuite")
		gotest.NoError(w, err)
		w.It("both suites run, the one binding the fixture against it", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "shared up")
			gotest.Equal(it, "pass", run.Verdict("TestBoundTestSuite/TestOne"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestFreeTestSuite/TestOne"))
		})
	})
}

func (s *FixtureSelectionTestSuite) TestBenchSkipFlag(t *gotest.T) {
	t.When("-skip names the only benchmark suite binding a shared fixture", func(w *gotest.T) {
		markers := w.TempDir()
		run, err := testkit.RunCLIWithEnv(w.Context(), ".", []string{"GOTEST_CANARY_DIR=" + markers}, "bench", "../testdata/skipped/", "-skip=BenchmarkBoundTestSuite", "-benchtime=1x")
		gotest.NoError(w, err)
		data, err := os.ReadFile(filepath.Join(markers, testkit.MarkersFile))
		if !errors.Is(err, fs.ErrNotExist) {
			gotest.NoError(w, err)
		}
		w.It("a bench run never starts the fixture and benchmarks the other suite", func(it *gotest.T) {
			gotest.NotContains(it, string(data), "shared up")
			gotest.Contains(it, run.Stdout, "BenchmarkFreeTestSuite/BenchmarkOne")
			gotest.NotContains(it, run.Stdout, "BenchmarkBoundTestSuite")
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
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
