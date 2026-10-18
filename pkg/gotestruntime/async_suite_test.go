package gotestruntime_test

import (
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// AsyncWaitTestSuite: an async method is held to a deadline only when its
// suite declared one, and a stop ends the wait as a stop, not as a missed
// done.
// Sequential: one method stops the run, which is process-wide.
type AsyncWaitTestSuite struct{}

func (s *AsyncWaitTestSuite) AfterEach(t *gotest.T) { gotestruntime.ExportResetStop() }

func (s *AsyncWaitTestSuite) TestNoDeclaredBudgetMeansNoDeadlineVerdict(t *gotest.T) {
	// The context carries the default deadline a suite without a config gets,
	// here a short one; no budget was declared.
	tt := gotestruntime.TestT(t.T(), 50*time.Millisecond)
	gotestruntime.AwaitDoneAt(tt, "TestLateAsync", "x_test.go:1", func(done func()) {
		go func() {
			time.Sleep(200 * time.Millisecond)
			done()
		}()
	})
	t.When("done arrives after the default context deadline and no Timeout was declared", func(w *gotest.T) {
		w.It("is not a failure", func(it *gotest.T) {
			gotest.False(it, t.T().Failed())
		})
	})
}

func (s *AsyncWaitTestSuite) TestAStopEndsTheWaitAsAStop(t *gotest.T) {
	var skipped, failed bool
	t.T().Run("method", func(mt *testing.T) { //nolint:suite-lifecycle // the wait runs as a subtest of a test of this suite's own
		defer func() { skipped, failed = mt.Skipped(), mt.Failed() }()
		tt := gotestruntime.TestT(mt, time.Minute)
		gotestruntime.AwaitDoneAt(tt, "TestHoldsAsync", "x_test.go:2", func(done func()) {
			go func() {
				time.Sleep(50 * time.Millisecond)
				runstate.Stop()
			}()
		})
	})
	t.When("the run is stopped while an async method waits for done", func(w *gotest.T) {
		w.It("skips the method as run stopping instead of failing it for a missed done", func(it *gotest.T) {
			gotest.True(it, skipped)
			gotest.False(it, failed)
		})
	})
}
