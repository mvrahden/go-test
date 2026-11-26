package gotest_test

import (
	"context"
	"errors"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BubbleTestSuite: code a Bubble runs reads the bubble's fake clock, and
// what the bubble starts is done when Bubble returns.
type BubbleTestSuite struct{}

func (s *BubbleTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// retry calls op until it succeeds, waiting a doubling delay from a second
// between calls, and returns how many calls it made.
func retry(ctx context.Context, op func() error) (calls int) {
	delay := time.Second
	for {
		calls++
		if op() == nil {
			return calls
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return calls
		}
		delay *= 2
	}
}

func (s *BubbleTestSuite) TestAnHourOfBackoff(t *gotest.T) {
	t.When("a retry loop backs off for over an hour in a bubble", func(w *gotest.T) {
		w.It("takes the hour on the fake clock and no real second", func(it *gotest.T) {
			start := time.Now()
			gotest.Bubble(it, func(bt *gotest.T) {
				begin := time.Now()
				calls := retry(bt.Context(), func() error {
					if time.Since(begin) < time.Hour {
						return errors.New("not yet")
					}
					return nil
				})
				gotest.Equal(bt, 13, calls) // the 13th, at 4095s, is the first past the hour
				gotest.Equal(bt, 4095*time.Second, time.Since(begin))
			})
			gotest.Less(it, time.Since(start), time.Second)
		})
	})
}

func (s *BubbleTestSuite) TestTheBubblesContext(t *gotest.T) {
	t.When("the bubble's T is asked for its context", func(w *gotest.T) {
		w.It("has no deadline, and one derived from it expires on the fake clock", func(it *gotest.T) {
			gotest.Bubble(it, func(bt *gotest.T) {
				_, ok := bt.Context().Deadline()
				gotest.False(bt, ok)
				begin := time.Now()
				bounded := gotest.NewTWithDeadline(bt, time.Hour)
				<-bounded.Context().Done()
				gotest.Equal(bt, time.Hour, time.Since(begin))
				gotest.ErrorIs(bt, bounded.Context().Err(), context.DeadlineExceeded)
			})
		})
		w.It("ends when the callback returns, before the waits on its gotest.Go goroutines", func(it *gotest.T) {
			var stopped bool
			gotest.Bubble(it, func(bt *gotest.T) {
				ctx := bt.Context()
				gotest.Go(bt, func() {
					<-ctx.Done()
					stopped = true
				})
			})
			gotest.True(it, stopped)
		})
	})
}

func (s *BubbleTestSuite) TestSettle(t *gotest.T) {
	t.When("a ticker goroutine counts while the callback sleeps", func(w *gotest.T) {
		w.It("Settle lets it count the last tick before the callback reads", func(it *gotest.T) {
			gotest.Bubble(it, func(bt *gotest.T) {
				ticks := 0
				ticker := time.NewTicker(time.Minute)
				defer ticker.Stop()
				ctx := bt.Context()
				gotest.Go(bt, func() {
					for {
						select {
						case <-ticker.C:
							ticks++
						case <-ctx.Done():
							return
						}
					}
				})
				time.Sleep(10 * time.Minute)
				gotest.Settle(bt)
				gotest.Equal(bt, 10, ticks)
			})
		})
	})
}

// BubbleHookTestSuite: a hook's T bubbles as a method's does.
type BubbleHookTestSuite struct {
	hooked time.Duration
}

func (s *BubbleHookTestSuite) BeforeEach(t *gotest.T) {
	gotest.Bubble(t, func(bt *gotest.T) {
		begin := time.Now()
		time.Sleep(time.Hour)
		s.hooked = time.Since(begin)
	})
}

func (s *BubbleHookTestSuite) TestInAHook(t *gotest.T) {
	t.When("BeforeEach sleeps an hour in a bubble", func(w *gotest.T) {
		w.It("the hour passed on the fake clock", func(it *gotest.T) {
			gotest.Equal(it, time.Hour, s.hooked)
		})
	})
}

func (s *BubbleTestSuite) FuzzInAFuzzBody(f *gotest.F) {
	f.Add(3)
	f.Add(0)
	f.Add(-2)
	f.Fuzz(func(t *gotest.T, n int) {
		gotest.Bubble(t, func(bt *gotest.T) {
			begin := time.Now() //nolint:fuzz-determinism // a bubble's clock starts at the same instant on every run
			// n%(1<<16) keeps n hours from overflowing a Duration; a negative
			// sleep returns at once.
			d := time.Duration(n%(1<<16)) * time.Hour
			time.Sleep(d)
			gotest.Equal(bt, max(d, 0), time.Since(begin))
		})
	})
}
