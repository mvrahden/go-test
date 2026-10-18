package assertions_test

import (
	"sync/atomic"
	"time"

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

func hang(*gotest.R) { time.Sleep(10 * time.Second) }

func (s *PollingTestSuite) TestHangingPoll(t *gotest.T) {
	t.When("an Eventually poll runs past waitFor", func(w *gotest.T) {
		w.It("fails no later than one tick past waitFor and says the poll is still running", func(it *gotest.T) {
			began := time.Now()
			m := gotest.Record(func(r *gotest.R) { gotest.Eventually(r, 200*time.Millisecond, 50*time.Millisecond, hang) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running after 200ms")
			gotest.Less(it, time.Since(began), 2*time.Second)
		})
	})
	t.When("a Consistently poll runs past waitFor", func(w *gotest.T) {
		w.It("fails no later than one tick past waitFor and says the poll is still running", func(it *gotest.T) {
			began := time.Now()
			m := gotest.Record(func(r *gotest.R) { gotest.Consistently(r, 200*time.Millisecond, 50*time.Millisecond, hang) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "poll still running after 200ms")
			gotest.Less(it, time.Since(began), 2*time.Second)
		})
	})
}

// slowPass passes after 700ms: started before a 500ms waitFor, it returns
// after it and within the 500ms tick that follows.
func slowPass(*gotest.R) { time.Sleep(700 * time.Millisecond) }

func (s *PollingTestSuite) TestLatePoll(t *gotest.T) {
	t.When("every Consistently poll passes but one runs past waitFor", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Consistently(r, 500*time.Millisecond, 500*time.Millisecond, slowPass) })
			gotest.False(it, m.Failed(), m.Message())
		})
	})
	t.When("an Eventually poll started before waitFor passes after it", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Eventually(r, 500*time.Millisecond, 500*time.Millisecond, slowPass) })
			gotest.False(it, m.Failed(), m.Message())
		})
	})
}

func (s *PollingTestSuite) TestFinalPoll(t *gotest.T) {
	t.When("tick is longer than waitFor and the first poll fails", func(w *gotest.T) {
		w.It("a final poll runs at the deadline and can still pass", func(it *gotest.T) {
			var polls atomic.Int32
			began := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.Eventually(r, 200*time.Millisecond, 10*time.Second, func(poll *gotest.R) {
					gotest.Greater(poll, polls.Add(1), int32(1), "the first poll fails")
				})
			})
			gotest.False(it, m.Failed(), m.Message())
			gotest.Equal(it, int32(2), polls.Load())
			gotest.Less(it, time.Since(began), 2*time.Second)
		})
	})
}

func (s *PollingTestSuite) TestArguments(t *gotest.T) {
	t.When("waitFor or tick is not positive", func(w *gotest.T) {
		w.It("fails with a type-guard message and polls nothing", func(it *gotest.T) {
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
			gotest.Equal(it, int32(0), polls.Load())
		})
	})
}
