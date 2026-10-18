package fuzz_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzSelectionTestSuite: -skip drops a suite's fuzz seed replays only when
// it names every function of the suite, as go test does.
type FuzzSelectionTestSuite struct{}

func (s *FuzzSelectionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// runSelect runs the fzselect package with args and reads its markers back.
func runSelect(t *gotest.T, args ...string) (testkit.Run, []string) {
	markerDir := t.TempDir()
	run, err := testkit.RunPackage(t.Context(), markerDir, "../testdata/fzselect/", args...)
	gotest.NoError(t, err)
	markers, err := testkit.ReadMarkers(filepath.Join(markerDir, "markers"))
	gotest.NoError(t, err)
	return run, markers
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
