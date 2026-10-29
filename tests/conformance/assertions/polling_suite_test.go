package assertions_test

import (
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PollingTestSuite: Eventually and Consistently hold their polls to the
// deadline they were given.
type PollingTestSuite struct{}

func (s *PollingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// hangingPoll blocks until release closes, or for 10s, so a poll that is
// waited out ends the test instead of hanging it; returned records that it
// came back.
func hangingPoll(release <-chan struct{}, returned *atomic.Bool) func(*gotest.R) {
	return func(*gotest.R) {
		select {
		case <-release:
		case <-time.After(10 * time.Second):
		}
		returned.Store(true)
	}
}

func (s *PollingTestSuite) TestHangingPoll(t *gotest.T) {
	t.When("an Eventually poll runs past waitFor", func(w *gotest.T) {
		w.It("fails no later than waitFor past the deadline and says the poll is still running past its deadline", func(it *gotest.T) {
			release := make(chan struct{})
			var returned atomic.Bool
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 200*time.Millisecond, 50*time.Millisecond, hangingPoll(release, &returned))
			})
			gotest.False(it, returned.Load(), "Eventually waited for the poll to return")
			close(release)
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running past its deadline")
		})
	})
	t.When("a Consistently poll runs past waitFor", func(w *gotest.T) {
		w.It("fails no later than waitFor past the deadline and says the poll is still running past its deadline", func(it *gotest.T) {
			release := make(chan struct{})
			var returned atomic.Bool
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 200*time.Millisecond, 50*time.Millisecond, hangingPoll(release, &returned))
			})
			gotest.False(it, returned.Load(), "Consistently waited for the poll to return")
			close(release)
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running past its deadline")
		})
	})
}

// lateWait is waitFor and tick for slowPass, which starts before the deadline
// and returns a quarter tick after it: well inside the tick that follows.
const lateWait = time.Second

func slowPass(*gotest.R) { time.Sleep(lateWait + lateWait/4) }

func (s *PollingTestSuite) TestLatePoll(t *gotest.T) {
	t.When("every Consistently poll passes but one runs past waitFor", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Consistently(r, lateWait, lateWait, slowPass) })
			gotest.False(it, m.Failed(), m.Message())
		})
	})
	t.When("an Eventually poll started before waitFor passes after it", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Eventually(r, lateWait, lateWait, slowPass) })
			gotest.False(it, m.Failed(), m.Message())
		})
	})
}

// slowWait and slowTick pace polls that take many ticks; slowFor ends one
// well inside the waitFor of grace past the deadline.
const (
	slowWait = time.Second
	slowTick = 50 * time.Millisecond
	slowFor  = 300 * time.Millisecond
)

func (s *PollingTestSuite) TestSlowPoll(t *gotest.T) {
	t.When("a passing Consistently poll in flight at the deadline takes many ticks", func(w *gotest.T) {
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
	t.When("the final Eventually poll passes after many ticks", func(w *gotest.T) {
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
}

func (s *PollingTestSuite) TestFinalPoll(t *gotest.T) {
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
	t.When("tick is longer than waitFor and every Consistently poll passes", func(w *gotest.T) {
		w.It("a final poll runs at the deadline", func(it *gotest.T) {
			var polls atomic.Int32
			m := gotest.Record(func(r *gotest.R) {
				gotest.Consistently(r, 200*time.Millisecond, time.Hour, func(*gotest.R) { polls.Add(1) })
			})
			gotest.False(it, m.Failed(), m.Message())
			gotest.Equal(it, int32(2), polls.Load())
		})
	})
}

func (s *PollingTestSuite) TestArguments(t *gotest.T) {
	t.When("waitFor or tick is not positive", func(w *gotest.T) {
		w.It("fails saying both must be positive and polls nothing", func(it *gotest.T) {
			var polls atomic.Int32
			count := func(*gotest.R) { polls.Add(1) }
			for _, c := range []struct {
				name          string
				waitFor, tick time.Duration
			}{{"zero tick", time.Second, 0}, {"zero waitFor", 0, 10 * time.Millisecond}, {"negative tick", time.Second, -1}} {
				e := gotest.Record(func(r *gotest.R) { gotest.Eventually(r, c.waitFor, c.tick, count) })
				gotest.True(it, e.Failed(), c.name)
				gotest.Contains(it, e.Message(), "must be positive", c.name)
				k := gotest.Record(func(r *gotest.R) { gotest.Consistently(r, c.waitFor, c.tick, count) })
				gotest.True(it, k.Failed(), c.name)
				gotest.Contains(it, k.Message(), "must be positive", c.name)
			}
			gotest.Zero(it, polls.Load())
		})
	})
}

// AbandonedPollTestSuite: a poll given up on keeps running, and what it
// raises later still reaches the reader.
type AbandonedPollTestSuite struct{}

func (s *AbandonedPollTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 45 * time.Second}
}

func (s *AbandonedPollTestSuite) TestLatePanic(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/latepanic/")
	gotest.NoError(t, err)
	t.When("a poll abandoned past its deadline panics later", func(w *gotest.T) {
		w.It("the panic is written to stderr with its stack", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestLatePanicTestSuite/TestAbandonedPollPanics"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "gotest: a poll abandoned past its deadline panicked: late boom")
			gotest.Contains(it, run.Stdout, "latepanic_suite_test.go:")
		})
	})
}
