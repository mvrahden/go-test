package gotest_test

import (
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AsyncAssertionsTestSuite covers the polling assertions.
type AsyncAssertionsTestSuite struct{}

func (s *AsyncAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AsyncAssertionsTestSuite) TestEventually(t *gotest.T) {
	t.When("condition becomes true before timeout", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			count := 0
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 50*time.Millisecond, 1*time.Millisecond, func(poll *gotest.R) {
					count++
					gotest.GreaterOrEqual(poll, count, 3)
				})
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes on first poll", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, true)
				})
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes for async goroutine condition", func(it *gotest.T) {
			var counter atomic.Int32
			go func() {
				time.Sleep(50 * time.Millisecond)
				counter.Store(42)
			}()
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 1*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
					gotest.Equal(poll, int32(42), counter.Load())
				})
			})
			gotest.False(it, m.Failed())
		})
	})

	t.When("condition never becomes true", func(w *gotest.T) {
		w.It("fails after timeout", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 10*time.Millisecond, 1*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, false)
				})
			})
			gotest.True(it, m.Failed())
		})
	})
}

func (s *AsyncAssertionsTestSuite) TestConsistently(t *gotest.T) {
	t.When("condition stays true for duration", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 20*time.Millisecond, 1*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, true)
				})
			})
			gotest.False(it, m.Failed())
		})
	})

	t.When("condition becomes false", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			count := 0
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
					count++
					gotest.Less(poll, count, 2)
				})
			})
			gotest.True(it, m.Failed())
		})
		w.It("fails on first poll", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, false)
				})
			})
			gotest.True(it, m.Failed())
		})
	})
}
