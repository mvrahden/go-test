package fuzz_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzHooksTestSuite: a fuzz execution's hooks pair the way a test method's
// do.
type FuzzHooksTestSuite struct{}

func (s *FuzzHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FuzzHooksTestSuite) TestBeforeEachLeaves(t *gotest.T) {
	markerDir := t.TempDir()
	run, err := testkit.RunPackage(t.Context(), markerDir, "../testdata/fuzzhooks/")
	gotest.NoError(t, err)
	markers, err := testkit.ReadMarkers(filepath.Join(markerDir, "markers"))
	gotest.NoError(t, err)
	t.When("a fuzz execution's BeforeEach fails", func(w *gotest.T) {
		w.It("the execution fails and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("FuzzBrokenBeforeTestSuite_FuzzOne/seed#0"), "stdout: %s", run.Stdout)
			gotest.Contains(it, markers, "after broken")
		})
	})
	t.When("a fuzz execution's BeforeEach panics", func(w *gotest.T) {
		w.It("the execution fails and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("FuzzPanicBeforeTestSuite_FuzzOne/seed#0"), "stdout: %s", run.Stdout)
			gotest.Contains(it, markers, "after panic")
		})
	})
}
