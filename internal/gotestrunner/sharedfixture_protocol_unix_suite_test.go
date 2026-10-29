//go:build unix

package gotestrunner_test

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureProtocolProcessTestSuite: a process that gets the protocol
// wrong after a good setup is still judged by how it exits.
type SharedFixtureProtocolProcessTestSuite struct{}

func (s *SharedFixtureProtocolProcessTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// twiceDone reports its setup done twice, then exits 1 on the shutdown request.
const twiceDone = `trap 'exit 1' TERM
printf '{"key":"_done","teardownBudget":"10s"}\n' >&3
sleep 0.2
printf '{"key":"_done","teardownBudget":"10s"}\n' >&3
while :; do sleep 0.05; done`

func (s *SharedFixtureProtocolProcessTestSuite) TestASecondDoneKeepsTheTeardownVerdict(t *gotest.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proc, err := gotestrunner.ExportLaunchSharedProcess(ctx, t.TempDir(), "sh", "-c", twiceDone)
	gotest.NoError(t, err, "starting the fake shared fixture process")
	waitErr := proc.WaitAllReady(ctx, 10*time.Second)
	time.Sleep(500 * time.Millisecond) // the second _done is read meanwhile
	setupErr := proc.SetupErr()
	teardownErr := proc.Teardown()

	t.When("the process reports _done twice after a good setup and fails its teardown", func(w *gotest.T) {
		w.It("the setup stays good", func(it *gotest.T) {
			gotest.NoError(it, waitErr)
			gotest.NoError(it, setupErr)
		})
		w.It("the teardown is judged by its exit status and names the protocol error", func(it *gotest.T) {
			gotest.ErrorContains(it, teardownErr, "teardown failed")
			gotest.ErrorContains(it, teardownErr, "_done twice")
		})
	})
}

// sameBudgetAck reports a 1s teardown budget, then answers the shutdown
// request 300ms later with an ack reporting the same budget, and keeps running.
const sameBudgetAck = `ack() { sleep 0.3; printf '{"key":"_cmd","id":99,"teardownBudget":"1s"}\n' >&3; }
trap ack TERM
printf '{"key":"_done","teardownBudget":"1s"}\n' >&3
while :; do sleep 0.05; done`

func (s *SharedFixtureProtocolProcessTestSuite) TestAnAckReportingTheSameBudgetExtendsNothing(t *gotest.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proc, err := gotestrunner.ExportLaunchSharedProcess(ctx, t.TempDir(), "sh", "-c", sameBudgetAck)
	gotest.NoError(t, err, "starting the fake shared fixture process")
	waitErr := proc.WaitAllReady(ctx, 10*time.Second)
	teardownErr := proc.Teardown()

	t.When("an ack arriving while the teardown waits reports the budget it already has", func(w *gotest.T) {
		w.It("the process is force-killed at that budget, not that budget after the ack", func(it *gotest.T) {
			gotest.NoError(it, waitErr)
			gotest.ErrorContains(it, teardownErr, "force-killed after 1s;")
		})
	})
}
