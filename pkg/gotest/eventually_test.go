package gotest_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestEventually_PassesWhenConditionMet(t *testing.T) {
	gt := gotest.NewT(t)

	var counter atomic.Int32

	go func() {
		time.Sleep(50 * time.Millisecond)
		counter.Store(42)
	}()

	gt.Eventually(1*time.Second, 10*time.Millisecond, func(poll *gotest.T) {
		gotest.Equal(poll, int32(42), counter.Load())
	})
}

func TestEventually_ImmediatePass(t *testing.T) {
	gt := gotest.NewT(t)

	gt.Eventually(1*time.Second, 10*time.Millisecond, func(poll *gotest.T) {
		gotest.True(poll, true)
	})
}

func TestConsistently_PassesWhenAlwaysTrue(t *testing.T) {
	gt := gotest.NewT(t)

	gt.Consistently(100*time.Millisecond, 20*time.Millisecond, func(poll *gotest.T) {
		gotest.True(poll, true)
	})
}
