package gotest_test

import (
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type EventuallyTestSuite struct{}

func (s *EventuallyTestSuite) TestEventually(t *gotest.T) {
	t.When("condition becomes true before timeout", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			var counter atomic.Int32
			go func() {
				time.Sleep(50 * time.Millisecond)
				counter.Store(42)
			}()
			it.Eventually(1*time.Second, 10*time.Millisecond, func(poll *gotest.T) {
				gotest.Equal(poll, int32(42), counter.Load())
			})
		})
	})

	t.When("condition is immediately true", func(w *gotest.T) {
		w.It("passes on first poll", func(it *gotest.T) {
			it.Eventually(1*time.Second, 10*time.Millisecond, func(poll *gotest.T) {
				gotest.True(poll, true)
			})
		})
	})
}

func (s *EventuallyTestSuite) TestConsistently(t *gotest.T) {
	t.When("condition stays true for the duration", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			it.Consistently(100*time.Millisecond, 20*time.Millisecond, func(poll *gotest.T) {
				gotest.True(poll, true)
			})
		})
	})
}
