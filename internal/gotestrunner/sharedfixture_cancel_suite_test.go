package gotestrunner_test

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureCancelTestSuite: a run's cancellation does not reach the shared
// fixture process. Its teardown has one owner, after every suite has stopped,
// so a suite finishing gracefully still has its fixtures.
// Sequential: the fixture is configured through the environment.
type SharedFixtureCancelTestSuite struct{}

func (s *SharedFixtureCancelTestSuite) TestCancellationLeavesTheProcessToTeardown(t *gotest.T) {
	t.Setenv(fixtures.EnvSlowSetupTimeout, "30s")
	t.Setenv(fixtures.EnvSlowSetupDelay, "0s")
	ctx, cancel := context.WithCancel(t.Context())
	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), slowSetupFixture(), 0)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	gotest.NoError(t, proc.WaitSetup(ctx, 0))
	cancel()
	t.When("the run's context is cancelled while the fixture is up", func(w *gotest.T) {
		w.It("leaves the process running until Teardown, which still succeeds", func(it *gotest.T) {
			gotest.Consistently(it, 1500*time.Millisecond, 100*time.Millisecond, func(poll *gotest.R) {
				select {
				case <-gotestrunner.ExportExited(proc):
					gotest.Fail(poll, "the cancellation ended the shared fixture process")
				default:
				}
			})
			gotest.NoError(it, proc.Teardown())
			select {
			case <-gotestrunner.ExportExited(proc):
			default:
				gotest.Fail(it, "Teardown returned with the process still running")
			}
		})
	})
}
