// Package bubble runs code in gotest.Bubble: on the fake clock, counted and
// located as the method, and refusing what a bubble cannot run.
package bubble

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BubbleTestSuite struct{}

func (s *BubbleTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 5 * time.Second}
}

// backoff waits d in doubling steps from a second, unless ctx ends first,
// and returns how long it waited.
func backoff(ctx context.Context, d time.Duration) time.Duration {
	start := time.Now()
	for step := time.Second; time.Since(start) < d; step *= 2 {
		select {
		case <-time.After(min(step, d-time.Since(start))):
		case <-ctx.Done():
			return time.Since(start)
		}
	}
	return time.Since(start)
}

func (s *BubbleTestSuite) TestBackoff(t *gotest.T) {
	start := time.Now()
	gotest.Bubble(t, func(bt *gotest.T) {
		gotest.Equal(bt, time.Hour, backoff(bt.Context(), time.Hour))
	})
	gotest.Less(t, time.Since(start), time.Second)
}

func (s *BubbleTestSuite) TestOnlyInside(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) {
		gotest.True(bt, true)
		gotest.True(bt.T(), true)
	})
}

func (s *BubbleTestSuite) TestFails(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) {
		gotest.Equal(bt, 1, 2) // fails here
	})
}

func (s *BubbleTestSuite) TestRows(t *gotest.T) {
	for row, c := range gotest.Each(t, []struct {
		Desc string
		Want int
	}{{"failing", 2}, {"passing", 1}}) {
		gotest.Bubble(row, func(bt *gotest.T) {
			gotest.Equal(bt, c.Want, 1)
		})
	}
}

// TestEventually polls on a row's T, which goes on after the failure to
// time the bubble.
func (s *BubbleTestSuite) TestEventually(t *gotest.T) {
	for row := range gotest.Each(t, []struct{ Desc string }{{"hour"}}) {
		start := time.Now()
		gotest.Bubble(row, func(bt *gotest.T) {
			gotest.Eventually(bt, time.Hour, time.Minute, func(poll *gotest.R) {
				gotest.True(poll, false, "never ready")
			})
		})
		gotest.Less(row, time.Since(start), time.Second)
	}
}

func (s *BubbleTestSuite) TestSettle(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) {
		value := 0
		go func() {
			for range 1000 {
				runtime.Gosched()
			}
			value = 1
		}()
		gotest.Settle(bt)
		gotest.Equal(bt, 1, value)
	})
}

func (s *BubbleTestSuite) TestIt(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) {
		bt.It("never runs", func(it *gotest.T) {}) // opens an It
	})
}

func (s *BubbleTestSuite) TestParallel(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) {
		bt.T().Parallel() // calls Parallel
	})
}

func (s *BubbleTestSuite) TestOuterT(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) {
		_ = t.Context() // asks the method's T
	})
}

func (s *BubbleTestSuite) TestDeadlock(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) { // deadlocks
		never := make(chan struct{})
		go func() { <-never }() // blocks forever
	})
}

func (s *BubbleTestSuite) TestSettleOutside(t *gotest.T) {
	gotest.Settle(t) // settles outside
}

// AbandonTestSuite holds a bubble on a mutex no goroutine in it can release.
type AbandonTestSuite struct{}

func (s *AbandonTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Second}
}

func (s *AbandonTestSuite) TestBlockedOutside(t *gotest.T) {
	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()
	gotest.Bubble(t, func(bt *gotest.T) { // is abandoned
		mu.Lock()
	})
}
