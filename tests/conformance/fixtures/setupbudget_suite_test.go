package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SetupBudgetTestSuite: the wait for shared fixtures is what the fixtures
// declared, unless a --setup-timeout says otherwise.
type SetupBudgetTestSuite struct{}

func (s *SetupBudgetTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	cfg.Timeout = 5 * time.Minute // the fixture under test takes 130s
	return cfg
}

func (s *SetupBudgetTestSuite) TestContainerConfig(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/slowsetup/")
	gotest.NoError(t, err)
	t.When("a shared fixture declaring ContainerFixtureConfig takes 130s to come up and no --setup-timeout is given", func(w *gotest.T) {
		w.It("the run passes: the wait is the fixture's declared budget", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestContainerTestSuite/TestStateArrived"), "stderr: %s", run.Stderr)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
		})
	})
}
