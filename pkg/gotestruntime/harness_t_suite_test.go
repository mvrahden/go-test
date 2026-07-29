package gotestruntime_test

import (
	"context"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// HarnessTTestSuite tests the lifecycle-phase T constructors that generated
// harnesses use for BeforeAll, test methods and AfterAll.
type HarnessTTestSuite struct{}

func (s *HarnessTTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *HarnessTTestSuite) TestSetupAndTestT(t *gotest.T) {
	t.When("a positive timeout is configured", func(w *gotest.T) {
		w.It("applies it as a deadline", func(it *gotest.T) {
			for _, tt := range []*gotest.T{
				gotestruntime.SetupT(it.T(), 5*time.Second),
				gotestruntime.TestT(it.T(), 5*time.Second),
			} {
				deadline, ok := tt.Context().Deadline()
				gotest.True(it, ok)
				remaining := time.Until(deadline)
				gotest.True(it, remaining > 0 && remaining <= 5*time.Second)
			}
		})

		w.It("expires on its own", func(it *gotest.T) {
			tt := gotestruntime.TestT(it.T(), 10*time.Millisecond)
			<-tt.Context().Done()
			gotest.ErrorIs(it, tt.Context().Err(), context.DeadlineExceeded)
		})

		w.It("preserves the original testing.T", func(it *gotest.T) {
			tt := gotestruntime.SetupT(it.T(), time.Second)
			gotest.Equal(it, it.T(), tt.T())
		})
	})

	t.When("the timeout is disabled", func(w *gotest.T) {
		for sub, timeout := range gotest.Each(w, []time.Duration{0, -1}) {
			tt := gotestruntime.TestT(sub.T(), timeout)
			_, ok := tt.Context().Deadline()
			gotest.False(sub, ok, "timeout %s should leave the context unbounded", timeout)
			gotest.NoError(sub, tt.Context().Err())
		}
	})
}

func (s *HarnessTTestSuite) TestTeardownT(t *gotest.T) {
	t.When("built from inside t.Cleanup", func(w *gotest.T) {
		w.It("survives the cancellation testing performs before cleanup", func(it *gotest.T) {
			// The testing package cancels t.Context() immediately before it runs
			// the cleanup functions, so a teardown T derived from it would reach
			// AfterAll already canceled.
			var teardownErr, derivedErr error
			// A raw subtest is required here: only *testing.T exposes Cleanup,
			// and the cancellation under test happens on the way into it.
			it.T().Run("inner", func(inner *testing.T) { //nolint:t-escape // observing testing.T cleanup semantics
				inner.Cleanup(func() {
					teardownErr = gotestruntime.TeardownT(inner, time.Minute).Context().Err()
					derivedErr = inner.Context().Err()
				})
			})
			gotest.Error(it, derivedErr, "expected testing to cancel t.Context() before cleanup")
			gotest.NoError(it, teardownErr, "teardown context must outlive the test's own context")
		})

		w.It("stays live with the timeout disabled too", func(it *gotest.T) {
			var teardownErr error
			it.T().Run("inner", func(inner *testing.T) { //nolint:t-escape // observing testing.T cleanup semantics
				inner.Cleanup(func() {
					teardownErr = gotestruntime.TeardownT(inner, 0).Context().Err()
				})
			})
			gotest.NoError(it, teardownErr, "an unbounded teardown context must still be live")
		})
	})

	t.When("a timeout is configured", func(w *gotest.T) {
		w.It("applies it as a deadline", func(it *gotest.T) {
			tt := gotestruntime.TeardownT(it.T(), 5*time.Second)
			deadline, ok := tt.Context().Deadline()
			gotest.True(it, ok)
			remaining := time.Until(deadline)
			gotest.True(it, remaining > 0 && remaining <= 5*time.Second)
		})

		w.It("expires on its own", func(it *gotest.T) {
			tt := gotestruntime.TeardownT(it.T(), 10*time.Millisecond)
			<-tt.Context().Done()
			gotest.ErrorIs(it, tt.Context().Err(), context.DeadlineExceeded)
		})
	})
}
