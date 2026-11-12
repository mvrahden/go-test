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
}
