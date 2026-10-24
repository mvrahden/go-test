package fuzz_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzHookSkipTestSuite: a fuzz execution whose BeforeEach skips pairs its
// hooks the way a test method does.
type FuzzHookSkipTestSuite struct{}

func (s *FuzzHookSkipTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: slowest method ~62s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute}
}

func (s *FuzzHookSkipTestSuite) TestBeforeEachSkips(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fzskiphook/")
	gotest.NoError(t, err)
	markers := run.Markers
	t.When("a fuzz execution's BeforeEach skips", func(w *gotest.T) {
		w.It("the execution skips, its body never runs and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "skip", run.Verdict("FuzzSkipBeforeTestSuite_FuzzOne/seed#0"), "stdout: %s", run.Stdout)
			gotest.NotContains(it, markers, "body ran")
			gotest.Contains(it, markers, "after skip")
		})
	})
}
