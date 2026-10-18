package gotestruntime_test

import (
	"fmt"
	"sync/atomic"
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

// runAsync runs one async method, with an AfterEach, under a declared
// Timeout, as a generated suite does.
func runAsync(t *testing.T, timeout time.Duration, start func(tt *gotest.T, done func()), afterEach func()) {
	budget := gotest.SuiteConfig{Timeout: timeout}
	gotestruntime.RunMethods(t, budget, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
		Name: "TestAsync",
		Run: func(ttt *gotest.T, run func(func())) {
			defer afterEach()
			run(func() {
				gotestruntime.AwaitDoneAt(ttt, "TestAsync", "x_test.go:3", func(done func()) { start(ttt, done) })
			})
		},
	}}})
}

func (s *AsyncWaitTestSuite) TestTheTimeoutHoldsTheHooksAfterTheWait(t *gotest.T) {
	if inChild() {
		runAsync(t.T(), 300*time.Millisecond,
			func(_ *gotest.T, done func()) { done() },
			func() { time.Sleep(time.Second) })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestTheTimeoutHoldsTheHooksAfterTheWait")
	t.When("done is called in time and AfterEach then overruns the declared Timeout", func(w *gotest.T) {
		w.It("fails the method for the overrun", func(it *gotest.T) {
			gotest.Contains(it, out, "exceeded its configured Timeout of 300ms and is still running")
		})
	})
}

func (s *AsyncWaitTestSuite) TestAnotherGoroutinesFailureLeavesTheOwnerItsWork(t *gotest.T) {
	if inChild() {
		var finished atomic.Bool
		runAsync(t.T(), 10*time.Second,
			func(tt *gotest.T, done func()) {
				go func() { gotest.Fail(tt, "helper fails") }()
				go func() {
					time.Sleep(300 * time.Millisecond)
					finished.Store(true)
					done()
				}()
			},
			func() { fmt.Printf("AfterEach ran, owner finished: %v\n", finished.Load()) })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestAnotherGoroutinesFailureLeavesTheOwnerItsWork")
	t.When("a goroutine other than the one that calls done fails fatally", func(w *gotest.T) {
		w.It("fails the method and waits for done before AfterEach runs", func(it *gotest.T) {
			gotest.Contains(it, out, "helper fails")
			gotest.Contains(it, out, "AfterEach ran, owner finished: true")
		})
	})
}

func (s *AsyncWaitTestSuite) TestTheOwnersFailureEndsTheWait(t *gotest.T) {
	if inChild() {
		began := time.Now()
		runAsync(t.T(), 10*time.Second,
			func(tt *gotest.T, done func()) {
				go func() { gotest.Fail(tt, "owner fails") }()
			},
			func() { fmt.Printf("AfterEach ran, within a second: %v\n", time.Since(began) < time.Second) })
		return
	}
	out := runChild(t, "TestAsyncWaitTestSuite", "TestTheOwnersFailureEndsTheWait")
	t.When("the goroutine that would call done fails fatally and none of the method's goroutines is left", func(w *gotest.T) {
		w.It("ends the wait at once, not at the Timeout", func(it *gotest.T) {
			gotest.Contains(it, out, "owner fails")
			gotest.Contains(it, out, "AfterEach ran, within a second: true")
		})
	})
}
