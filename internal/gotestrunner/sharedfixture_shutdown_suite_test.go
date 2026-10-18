package gotestrunner_test

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureShutdownTestSuite: a setup the runner gave up on still leaves
// its healthy siblings the teardown budget the process reports.
// Sequential: the fixtures are configured through the environment.
type SharedFixtureShutdownTestSuite struct{}

// SuiteConfig: Exclusive, as every suite that builds and runs a real shared
// fixture subprocess against wall-clock budgets is.
func (s *SharedFixtureShutdownTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *SharedFixtureShutdownTestSuite) TestSiblingsGetTheReportedBudgetAfterASetupTimeout(t *gotest.T) {
	// The wait gives up at the sibling's 3s budget, and the runner allows a
	// process that has not reported its teardown budget 5s more. The hung
	// setup returns within that, at 6.5s, and the sibling's release then
	// outlasts it, ending at 9s.
	t.Setenv(fixtures.EnvSlowSetupTimeout, time.Second.String())
	t.Setenv(fixtures.EnvSlowSetupDelay, (6500 * time.Millisecond).String())
	t.Setenv(fixtures.EnvBudgetedTeardownTimeout, (3 * time.Second).String())
	t.Setenv(fixtures.EnvBudgetedTeardownDelay, (2500 * time.Millisecond).String())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), append(slowSetupFixture(), budgetedTeardownFixture()...), 0)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	waitErr := proc.WaitSetup(ctx, 0)
	teardownErr := proc.Teardown()

	t.When("one fixture outlives its declared setup budget and a sibling that came up is still to be released", func(w *gotest.T) {
		w.It("the wait is a setup timeout", func(it *gotest.T) {
			gotest.ErrorContains(it, waitErr, "timed out")
		})
		w.It("the process is given the teardown budget it reports once its setup returns", func(it *gotest.T) {
			gotest.NoError(it, teardownErr, "the process was force-killed before its teardown could run")
		})
	})
}
