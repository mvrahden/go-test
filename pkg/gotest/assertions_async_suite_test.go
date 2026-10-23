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

// latePollWait is waitFor and tick for a poll that starts before the deadline
// and ends after it; latePoll overshoots the deadline by a quarter of the
// tick, so the tick that follows leaves it ample room.
const latePollWait = time.Second

func latePoll() { time.Sleep(latePollWait + latePollWait/4) }

// waitOrGiveUp blocks until release closes, or for 5s, so a poll that is
// waited out ends the test instead of hanging it.
func waitOrGiveUp(release <-chan struct{}) {
	select {
	case <-release:
	case <-time.After(5 * time.Second):
	}
}

// slowWait and slowTick pace polls that each take many ticks: slowFor of
// them ends a poll well inside the waitFor of grace it gets past the
// deadline, and well past one tick.
const (
	slowWait = time.Second
	slowTick = 50 * time.Millisecond
	slowFor  = 300 * time.Millisecond
)

var badPollArgs = []struct {
	name          string
	waitFor, tick time.Duration
}{{"zero tick", time.Second, 0}, {"zero waitFor", 0, time.Millisecond}, {"negative tick", time.Second, -1}}

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
			var polls atomic.Int32
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, latePollWait, latePollWait, func(*gotest.R) { polls.Add(1); latePoll() })
			})
			gotest.False(it, m.Failed(), m.Message())
			gotest.Equal(it, int32(1), polls.Load())
		})
	})

	t.When("the final poll passes after many ticks", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			start := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, slowWait, slowTick, func(poll *gotest.R) {
					ready := time.Since(start) >= slowWait
					if ready {
						time.Sleep(slowFor)
					}
					gotest.True(poll, ready)
				})
			})
			gotest.False(it, m.Failed(), m.Message())
		})
	})

	t.When("a poll never returns", func(w *gotest.T) {
		w.It("fails as still running, without waiting for it", func(it *gotest.T) {
			release := make(chan struct{})
			var returned atomic.Bool
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 20*time.Millisecond, 10*time.Millisecond, func(*gotest.R) { waitOrGiveUp(release); returned.Store(true) })
			})
			gotest.False(it, returned.Load(), "Eventually waited for the poll")
			close(release)
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running past its deadline")
		})
	})

	t.When("tick is longer than waitFor and the first poll fails", func(w *gotest.T) {
		w.It("a final poll runs at the deadline and can still pass", func(it *gotest.T) {
			var polls atomic.Int32
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 200*time.Millisecond, time.Hour, func(poll *gotest.R) {
					gotest.Greater(poll, polls.Add(1), int32(1), "the first poll fails")
				})
			})
			gotest.False(it, m.Failed(), m.Message())
			gotest.Equal(it, int32(2), polls.Load())
		})
	})

	t.When("waitFor or tick is not positive", func(w *gotest.T) {
		w.It("fails without polling", func(it *gotest.T) {
			for _, c := range badPollArgs {
				var polls atomic.Int32
				m := gotest.Record(func(r *gotest.R) { gotest.Eventually(r, c.waitFor, c.tick, func(*gotest.R) { polls.Add(1) }) })
				gotest.True(it, m.Failed(), c.name)
				gotest.Contains(it, m.Message(), "must be positive", c.name)
				gotest.Zero(it, polls.Load(), c.name)
			}
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
		w.It("passes, and polls no more", func(it *gotest.T) {
			var polls atomic.Int32
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, latePollWait, latePollWait, func(*gotest.R) { polls.Add(1); latePoll() })
			})
			gotest.False(it, m.Failed(), m.Message())
			gotest.Equal(it, int32(1), polls.Load())
		})
	})

	t.When("a passing poll in flight at the deadline takes many ticks", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			start := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, slowWait, slowTick, func(*gotest.R) {
					if time.Since(start) > slowWait/2 {
						time.Sleep(time.Until(start.Add(slowWait + slowFor)))
					}
				})
			})
			gotest.False(it, m.Failed(), m.Message())
		})
	})

	t.When("a poll never returns", func(w *gotest.T) {
		w.It("fails as still running, without waiting for it", func(it *gotest.T) {
			release := make(chan struct{})
			var returned atomic.Bool
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 20*time.Millisecond, 10*time.Millisecond, func(*gotest.R) { waitOrGiveUp(release); returned.Store(true) })
			})
			gotest.False(it, returned.Load(), "Consistently waited for the poll")
			close(release)
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running past its deadline")
		})
	})

	t.When("tick is longer than waitFor", func(w *gotest.T) {
		w.It("a final poll runs at the deadline", func(it *gotest.T) {
			var polls atomic.Int32
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 200*time.Millisecond, time.Hour, func(*gotest.R) { polls.Add(1) })
			})
			gotest.False(it, m.Failed(), m.Message())
			gotest.Equal(it, int32(2), polls.Load())
		})
	})

	t.When("waitFor or tick is not positive", func(w *gotest.T) {
		w.It("fails without polling", func(it *gotest.T) {
			for _, c := range badPollArgs {
				var polls atomic.Int32
				m := gotest.Record(func(r *gotest.R) { gotest.Consistently(r, c.waitFor, c.tick, func(*gotest.R) { polls.Add(1) }) })
				gotest.True(it, m.Failed(), c.name)
				gotest.Contains(it, m.Message(), "must be positive", c.name)
				gotest.Zero(it, polls.Load(), c.name)
			}
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
