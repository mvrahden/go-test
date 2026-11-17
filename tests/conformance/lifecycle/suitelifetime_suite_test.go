package lifecycle_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SuiteLifetimeTestSuite: how long what BeforeAll takes from its T lasts.
type SuiteLifetimeTestSuite struct{}

func (s *SuiteLifetimeTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *SuiteLifetimeTestSuite) TestWhatBeforeAllTakes(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/suitelifetime/")
	gotest.NoError(t, err)
	t.When("BeforeAll takes a temporary directory and sets an environment variable through its T", func(w *gotest.T) {
		w.It("AfterAll still finds both", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestResourcesTestSuite"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "afterall: dir kept")
			gotest.Contains(it, run.Markers, "afterall: env kept")
		})
	})
	t.When("a suite's methods run past its SetupTimeout", func(w *gotest.T) {
		w.It("the context BeforeAll got is still live in them", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestLongTestSuite/TestOutlastsSetup"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Markers, "method: context live")
		})
		w.It("that context has ended by the time AfterAll runs", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "afterall: context ended")
		})
	})
	t.When("BeforeAll waits on its context past the SetupTimeout", func(w *gotest.T) {
		w.It("its context ends at the SetupTimeout", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "beforeall: context ended")
			gotest.Equal(it, "fail", run.Verdict("TestWaitingTestSuite"), "a declared SetupTimeout is held by verdict")
			gotest.Contains(it, run.Stdout, "BeforeAll exceeded its configured SetupTimeout of 300ms")
		})
	})
}
