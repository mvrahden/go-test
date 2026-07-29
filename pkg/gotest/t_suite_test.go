package gotest_test

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TTestSuite tests the T wrapper: context access and fallback to the
// underlying testing.T.
type TTestSuite struct{}

func (s *TTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *TTestSuite) TestNewTWithDeadline(t *gotest.T) {
	t.When("a deadline is set", func(w *gotest.T) {
		w.It("sets the context deadline", func(it *gotest.T) {
			tt := gotest.NewTWithDeadline(it.T(), 5*time.Second)
			deadline, ok := tt.Context().Deadline()
			gotest.True(it, ok)
			remaining := time.Until(deadline)
			gotest.True(it, remaining > 0 && remaining <= 5*time.Second)
		})

		w.It("cancels the context on timeout", func(it *gotest.T) {
			tt := gotest.NewTWithDeadline(it.T(), 10*time.Millisecond)
			<-tt.Context().Done()
			gotest.ErrorIs(it, tt.Context().Err(), context.DeadlineExceeded)
		})

		w.It("preserves the original testing.T", func(it *gotest.T) {
			tt := gotest.NewTWithDeadline(it.T(), 1*time.Second)
			gotest.Equal(it, it.T(), tt.T())
		})
	})
}

func (s *TTestSuite) TestNewTWithContext(t *gotest.T) {
	t.When("a context is supplied", func(w *gotest.T) {
		w.It("reports that context verbatim", func(it *gotest.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tt := gotest.NewTWithContext(it.T(), ctx)
			gotest.Equal(it, ctx, tt.Context())

			deadline, ok := tt.Context().Deadline()
			gotest.True(it, ok)
			remaining := time.Until(deadline)
			gotest.True(it, remaining > 0 && remaining <= 5*time.Second)
		})

		w.It("preserves the original testing.T", func(it *gotest.T) {
			tt := gotest.NewTWithContext(it.T(), context.Background())
			gotest.Equal(it, it.T(), tt.T())
		})

		w.It("does not cancel the caller's context", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tt := gotest.NewTWithContext(it.T(), ctx)
			gotest.NoError(it, tt.Context().Err())
		})
	})
}

func (s *TTestSuite) TestTContext(t *gotest.T) {
	t.When("custom ctx is set via NewTWithContext", func(w *gotest.T) {
		w.It("uses the custom ctx", func(it *gotest.T) {
			tt := gotest.NewTWithContext(it.T(), context.Background())
			gotest.NotZero(it, gotest.ExportTCtx(tt))
			gotest.Equal(it, gotest.ExportTCtx(tt), tt.Context())
		})
	})

	t.When("NewT is used without a context", func(w *gotest.T) {
		w.It("falls back to testing.T.Context()", func(it *gotest.T) {
			tt := gotest.NewT(it.T())
			gotest.Zero(it, gotest.ExportTCtx(tt))
			gotest.Equal(it, it.T().Context(), tt.Context())
		})
	})
}
