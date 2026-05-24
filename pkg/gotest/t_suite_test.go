package gotest_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

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
			gotest.True(it, gotest.ExportTCtx(tt) != nil)
			gotest.Equal(it, gotest.ExportTCtx(tt), tt.Context())
		})
	})

	t.When("NewT is used without deadline", func(w *gotest.T) {
		w.It("falls back to testing.T.Context()", func(it *gotest.T) {
			tt := gotest.NewT(it.T())
			gotest.True(it, gotest.ExportTCtx(tt) == nil)
			gotest.Equal(it, it.T().Context(), tt.Context())
		})
	})
}
