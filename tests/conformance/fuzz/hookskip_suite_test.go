package fuzz_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzHookSkipTestSuite: a fuzz execution whose BeforeEach skips pairs its
// hooks the way a test method does.
type FuzzHookSkipTestSuite struct{}

func (s *FuzzHookSkipTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FuzzHookSkipTestSuite) TestBeforeEachSkips(t *gotest.T) {
	markerDir := t.TempDir()
	run, err := testkit.RunPackage(t.Context(), markerDir, "../testdata/fzskiphook/")
	gotest.NoError(t, err)
	markers, err := testkit.ReadMarkers(filepath.Join(markerDir, "markers"))
	gotest.NoError(t, err)
	t.When("a fuzz execution's BeforeEach skips", func(w *gotest.T) {
		w.It("the execution skips, its body never runs and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "skip", run.Verdict("FuzzSkipBeforeTestSuite_FuzzOne/seed#0"), "stdout: %s", run.Stdout)
			gotest.NotContains(it, markers, "body ran")
			gotest.Contains(it, markers, "after skip")
		})
	})
}
