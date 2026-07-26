package gotest_test

import (
	"context"
	"testing"
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

func (s *TTestSuite) TestNestedBehaviorContext(t *gotest.T) {
	t.When("the enclosing T carries a deadline", func(w *gotest.T) {
		w.It("propagates it into When, It and Each", func(it *gotest.T) {
			outer := gotest.NewTWithDeadline(it.T(), time.Hour)
			outerDeadline, ok := outer.Context().Deadline()
			gotest.True(it, ok)

			seen := 0
			outer.When("nested when", func(nw *gotest.T) {
				deadline, ok := nw.Context().Deadline()
				gotest.True(nw, ok, "When must inherit the enclosing deadline")
				gotest.Equal(nw, outerDeadline, deadline)
				seen++

				nw.It("nested it", func(ni *gotest.T) {
					deadline, ok := ni.Context().Deadline()
					gotest.True(ni, ok, "It must inherit the enclosing deadline")
					gotest.Equal(ni, outerDeadline, deadline)
					seen++
				})

				for sub, entry := range gotest.Each(nw, []string{"only"}) {
					deadline, ok := sub.Context().Deadline()
					gotest.True(sub, ok, "Each must inherit the enclosing deadline: %s", entry)
					gotest.Equal(sub, outerDeadline, deadline)
					seen++
				}
			})
			gotest.Equal(it, 3, seen, "every nested form should have run")
		})

		w.It("still ends the nested context with the nested subtest", func(it *gotest.T) {
			outer := gotest.NewTWithDeadline(it.T(), time.Hour)

			var nested context.Context
			outer.It("nested it", func(ni *gotest.T) {
				nested = ni.Context()
				gotest.NoError(ni, nested.Err())
			})

			gotest.ErrorIs(it, nested.Err(), context.Canceled,
				"the nested context must not outlive its subtest")
			gotest.NoError(it, outer.Context().Err(), "cancelling a child must not cancel the parent")
		})
	})

	t.When("the enclosing T carries no explicit context", func(w *gotest.T) {
		w.It("falls back to the nested subtest's own context", func(it *gotest.T) {
			plain := gotest.NewT(it.T())
			plain.It("nested it", func(ni *gotest.T) {
				gotest.Zero(ni, gotest.ExportTCtx(ni))
				gotest.NoError(ni, ni.Context().Err())
			})
		})
	})
}

func (s *TTestSuite) TestNewTFromTB(t *gotest.T) {
	t.It("routes helpers through the TB and nils T()", func(it *gotest.T) {
		res := testing.Benchmark(func(b *testing.B) {
			tb := gotest.NewTFromTB(b)
			gotest.Zero(it, tb.T())
			gotest.NotZero(it, tb.Context())
			gotest.Equal(it, "recovered", func() (r string) {
				defer func() {
					if recover() != nil {
						r = "recovered"
					}
				}()
				tb.It("nope", func(*gotest.T) {})
				return "no panic"
			}())
			for b.Loop() {
			}
		})
		gotest.Greater(it, res.N, 0)
	})
}
