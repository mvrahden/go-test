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
	cfg.Timeout = 2 * time.Minute // the fixture under test takes 35s
	return cfg
}

func (s *SetupBudgetTestSuite) TestContainerConfig(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/slowsetup/")
	gotest.NoError(t, err)
	t.When("a shared fixture declaring ContainerFixtureConfig takes 35s to come up and no --setup-timeout is given", func(w *gotest.T) {
		w.It("the run passes: the wait is the fixture's declared budget", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestContainerTestSuite/TestStateArrived"), "stderr: %s", run.Stderr)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
		})
	})
}

func (s *SetupBudgetTestSuite) TestDeclaredBudgetBounds(t *gotest.T) {
	// Without a bound the fixture's 10-minute hang would outlast the run.
	began := time.Now()
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixahang/")
	elapsed := time.Since(began)
	gotest.NoError(t, err)
	t.When("a shared fixture declaring a 2s Timeout hangs in BeforeAll and no --setup-timeout is given", func(w *gotest.T) {
		w.It("fails the run within 60s, saying the setup timed out", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Less(it, elapsed, 60*time.Second)
			gotest.Regexp(it, `shared fixture setup timed out after 2s\b`, run.Stderr)
		})
	})
}
