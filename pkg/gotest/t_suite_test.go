package gotest_test

import (
	"context"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TTestSuite tests the T wrapper: deadline propagation, context access,
// and fallback to the underlying testing.T.
type TTestSuite struct{}

func (s *TTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *TTestSuite) TestNewTWithDeadline(t *gotest.T) {
	t.When("deadline is set", func(w *gotest.T) {
		w.It("sets context deadline", func(it *gotest.T) {
			tt := gotest.NewTWithDeadline(it.T(), 5*time.Second)
			deadline, ok := tt.Context().Deadline()
			gotest.True(it, ok)
			remaining := time.Until(deadline)
			gotest.True(it, remaining > 0 && remaining <= 5*time.Second)
		})

		w.It("context is cancelled on timeout", func(it *gotest.T) {
			tt := gotest.NewTWithDeadline(it.T(), 10*time.Millisecond)
			<-tt.Context().Done()
			gotest.Error(it, tt.Context().Err())
		})

		w.It("preserves the original testing.T", func(it *gotest.T) {
			tt := gotest.NewTWithDeadline(it.T(), 1*time.Second)
			gotest.Equal(it, it.T(), tt.T())
		})
	})
}

func (s *TTestSuite) TestTContext(t *gotest.T) {
	t.When("custom ctx is set via NewTWithDeadline", func(w *gotest.T) {
		w.It("uses the custom ctx", func(it *gotest.T) {
			tt := gotest.NewTWithDeadline(it.T(), 1*time.Second)
			gotest.NotZero(it, gotest.ExportTCtx(tt))
			gotest.Equal(it, gotest.ExportTCtx(tt), tt.Context())
		})
	})

	t.When("NewT is used without deadline", func(w *gotest.T) {
		w.It("falls back to testing.T.Context()", func(it *gotest.T) {
			tt := gotest.NewT(it.T())
			gotest.Zero(it, gotest.ExportTCtx(tt))
			gotest.Equal(it, it.T().Context(), tt.Context())
		})
	})
}

func (s *TTestSuite) TestNewTeardownT(t *gotest.T) {
	t.When("built from inside t.Cleanup", func(w *gotest.T) {
		w.It("survives the cancellation testing performs before cleanup", func(it *gotest.T) {
			// The testing package cancels t.Context() immediately before it runs
			// the cleanup functions, so a teardown T derived from it would reach
			// AfterAll already canceled.
			var teardownErr error
			var derivedErr error
			// A raw subtest is required here: only *testing.T exposes Cleanup,
			// and the cancellation under test happens on the way into it.
			it.T().Run("inner", func(inner *testing.T) { //nolint:t-escape // observing testing.T cleanup semantics
				inner.Cleanup(func() {
					teardownErr = gotest.NewTeardownT(inner, time.Minute).Context().Err()
					derivedErr = inner.Context().Err()
				})
			})
			gotest.Error(it, derivedErr, "expected testing to cancel t.Context() before cleanup")
			gotest.NoError(it, teardownErr, "teardown context must outlive the test's own context")
		})
	})

	t.When("a timeout is configured", func(w *gotest.T) {
		w.It("applies it as a deadline", func(it *gotest.T) {
			tt := gotest.NewTeardownT(it.T(), 5*time.Second)
			deadline, ok := tt.Context().Deadline()
			gotest.True(it, ok)
			remaining := time.Until(deadline)
			gotest.True(it, remaining > 0 && remaining <= 5*time.Second)
		})

		w.It("expires on its own", func(it *gotest.T) {
			tt := gotest.NewTeardownT(it.T(), 10*time.Millisecond)
			<-tt.Context().Done()
			gotest.ErrorIs(it, tt.Context().Err(), context.DeadlineExceeded)
		})
	})

	t.When("no timeout is configured", func(w *gotest.T) {
		w.It("hands over an unbounded but live context", func(it *gotest.T) {
			tt := gotest.NewTeardownT(it.T(), 0)
			_, ok := tt.Context().Deadline()
			gotest.False(it, ok, "no deadline should be set")
			gotest.NoError(it, tt.Context().Err())
		})
	})
}
