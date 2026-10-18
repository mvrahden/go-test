package fixtures_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureProtocolTestSuite: what a shared fixture's setup writes or
// holds never corrupts the state the runner hands to the suites.
type SharedFixtureProtocolTestSuite struct{}

func (s *SharedFixtureProtocolTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SharedFixtureProtocolTestSuite) TestSetupOutputAndState(t *gotest.T) {
	// A protocol that stalls would otherwise wait out the default setup budget.
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/protocol/", "--setup-timeout=20s")
	gotest.NoError(t, err)
	t.When("a shared fixture's setup prints to stdout, logs a JSON-shaped line and carries 2 MiB of state", func(w *gotest.T) {
		w.It("the suites receive the state and the run passes", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestStdoutTestSuite/TestStateArrived"))
			gotest.Equal(it, "pass", run.Verdict("TestBigTestSuite/TestStateArrived"))
			gotest.Equal(it, "pass", run.Verdict("TestLogTestSuite/TestStateArrived"))
		})
		w.It("the JSON-shaped log line is not an event of the run", func(it *gotest.T) {
			for _, ev := range run.Events {
				gotest.NotEqual(it, "TestBogus", ev.Test)
			}
		})
	})
}
