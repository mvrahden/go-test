package gotestrunner_test

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureWindowTestSuite: a fixture started late, for the exclusive
// tail or a bench slot, is waited for as the up-front phase is: bounded by
// the budget it declared, and given up when the run is stopped.
// Sequential: the fixture is configured through the environment.
type SharedFixtureWindowTestSuite struct{}

// SuiteConfig: Exclusive, as every suite that builds and runs a real shared
// fixture subprocess against wall-clock budgets is.
func (s *SharedFixtureWindowTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

const slowSetupKey = "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.SlowSetupSharedFixture"

// startDeferredSlowSetup boots the subprocess with the slow fixture held for
// a window, declaring timeout and taking delay once started. The returned
// release tears the process down without waiting the setup out.
func startDeferredSlowSetup(t *gotest.T, timeout, delay time.Duration) (proc *gotestrunner.SharedFixtureProcess, release func()) {
	t.Setenv(fixtures.EnvSlowSetupTimeout, timeout.String())
	t.Setenv(fixtures.EnvSlowSetupDelay, delay.String())
	deferred := slowSetupFixture()
	deferred[0].Deferred = true
	ctx, cancel := context.WithCancel(t.Context())
	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), deferred, 0)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	gotest.NoError(t, proc.WaitAllReady(ctx, 0), "waiting for the up-front phase")
	return proc, func() {
		gotestrunner.ExportSetTeardownTimeout(proc, time.Second)
		_ = proc.Teardown()
		cancel()
	}
}

func (s *SharedFixtureWindowTestSuite) TestAWindowIsHeldToTheDeclaredBudget(t *gotest.T) {
	proc, release := startDeferredSlowSetup(t, time.Second, 30*time.Second)
	defer release()
	began := time.Now()
	err := proc.StartKeys(t.Context(), []string{slowSetupKey}, 0)
	took := time.Since(began)
	t.When("no --setup-timeout is given and a fixture started for a window outlives the Timeout it declared", func(w *gotest.T) {
		w.It("the wait ends at the declared budget as a setup timeout", func(it *gotest.T) {
			gotest.ErrorContains(it, err, "timed out")
			gotest.Less(it, took, 10*time.Second)
		})
		w.It("a later command is refused at once while the process is still busy", func(it *gotest.T) {
			began := time.Now()
			gotest.ErrorContains(it, proc.TeardownKeys(it.Context(), []string{slowSetupKey}, time.Minute), "busy")
			gotest.Less(it, time.Since(began), time.Second)
		})
	})
}

func (s *SharedFixtureWindowTestSuite) TestAStoppedRunEndsTheWaitForAWindow(t *gotest.T) {
	proc, release := startDeferredSlowSetup(t, time.Minute, 30*time.Second)
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	began := time.Now()
	err := proc.StartKeys(ctx, []string{slowSetupKey}, -1)
	took := time.Since(began)
	t.When("the run is stopped while a fixture started for a window hangs, with --setup-timeout 0", func(w *gotest.T) {
		w.It("the wait ends with the run", func(it *gotest.T) {
			gotest.ErrorIs(it, err, context.DeadlineExceeded)
			gotest.Less(it, took, 5*time.Second)
		})
	})
}
