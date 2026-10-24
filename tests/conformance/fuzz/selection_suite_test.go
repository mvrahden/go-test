package fuzz_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzSelectionTestSuite: -skip drops a suite's fuzz seed replays only when
// it names every function of the suite, as go test does.
type FuzzSelectionTestSuite struct{}

func (s *FuzzSelectionTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: slowest method ~63s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute}
}

// runSelect runs the fzselect package with args and returns the run and its
// markers.
func runSelect(t *gotest.T, args ...string) (testkit.Run, []string) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fzselect/", args...)
	gotest.NoError(t, err)
	return run, run.Markers
}

func (s *FuzzSelectionTestSuite) TestSkipTheTestFunction(t *gotest.T) {
	run, markers := runSelect(t, "-skip=^TestMixedTestSuite$")
	t.When("-skip names a suite's Test function and the suite has fuzz methods", func(w *gotest.T) {
		w.It("its fuzz seeds still replay and pass", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("FuzzMixedTestSuite_FuzzOne/seed#0"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, markers, "shared up")
			gotest.Contains(it, markers, "seed ran")
		})
		w.It("its test method does not run", func(it *gotest.T) {
			gotest.Empty(it, run.Verdict("TestMixedTestSuite/TestOne"))
			gotest.NotContains(it, markers, "test ran")
		})
	})
}

func (s *FuzzSelectionTestSuite) TestSkipEveryFunction(t *gotest.T) {
	run, markers := runSelect(t, "-skip=^TestMixedTestSuite$|^FuzzMixedTestSuite_FuzzOne$")
	t.When("-skip names every Test and Fuzz function of a suite", func(w *gotest.T) {
		w.It("the suite does not run: its shared fixture never starts and the run passes", func(it *gotest.T) {
			gotest.Empty(it, markers)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
}

func (s *FuzzSelectionTestSuite) TestSuiteWithoutTestMethods(t *gotest.T) {
	t.When("-skip names every fuzz wrapper of a suite with no test method", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fzonly/", "-skip=^FuzzSeedsTestSuite_FuzzOne$|^FuzzSeedsTestSuite_FuzzTwo$")
		gotest.NoError(w, err)
		w.It("the suite does not run: its shared fixture never starts and the run passes", func(it *gotest.T) {
			gotest.Empty(it, run.Markers)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
	t.When("-run names only the Test function of a suite with no test method", func(w *gotest.T) {
		run, err := testkit.RunPackage(w.Context(), w.TempDir(), "../testdata/fzonly/", "-run=^TestSeedsTestSuite$")
		gotest.NoError(w, err)
		w.It("no seed replays and its shared fixture never starts, as the suite has no such function", func(it *gotest.T) {
			gotest.NotContains(it, run.Markers, "seed ran")
			gotest.NotContains(it, run.Markers, "shared up")
			gotest.Empty(it, run.Verdict("FuzzSeedsTestSuite_FuzzOne"))
		})
	})
}
