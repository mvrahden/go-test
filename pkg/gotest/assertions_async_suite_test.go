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

	t.When("a poll started before the deadline passes after it", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			var calls atomic.Int32
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 60*time.Millisecond, 40*time.Millisecond, func(poll *gotest.R) {
					gotest.NotEqual(poll, 1, calls.Add(1)) // the first poll fails
					time.Sleep(40 * time.Millisecond)      // ends past the deadline, inside one tick of it
				})
			})
			gotest.False(it, m.Failed(), m.Message())
		})
	})

	t.When("a poll never returns", func(w *gotest.T) {
		w.It("fails as still running", func(it *gotest.T) {
			release := make(chan struct{})
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 20*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) { <-release })
			})
			close(release)
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running")
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

	t.When("every poll passes but one runs past the deadline", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 100*time.Millisecond, 30*time.Millisecond, func(poll *gotest.R) {
					time.Sleep(40 * time.Millisecond)
				})
			})
			gotest.False(it, m.Failed(), m.Message())
		})
	})

	t.When("a poll never returns", func(w *gotest.T) {
		w.It("fails as still running", func(it *gotest.T) {
			release := make(chan struct{})
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 20*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) { <-release })
			})
			close(release)
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running")
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
