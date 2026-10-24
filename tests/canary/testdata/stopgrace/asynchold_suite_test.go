package stopgrace

import (
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AsyncHoldTestSuite's async methods wait for a done their owner never
// calls, one after a helper goroutine failed; only a stop ends the waits.
// "running" is marked once both wait, well after the failure: a wait that
// ended at the failure has run its AfterEach by then.
type AsyncHoldTestSuite struct{}

func (s *AsyncHoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// waiting counts the methods whose owner of done has started.
var waiting sync.WaitGroup

func (s *AsyncHoldTestSuite) BeforeAll(t *gotest.T) { waiting.Add(2) }
func (s *AsyncHoldTestSuite) AfterAll(t *gotest.T)  { mark("async AfterAll") }

func (s *AsyncHoldTestSuite) AfterEach(t *gotest.T) { mark("async AfterEach " + t.T().Name()) }

func (s *AsyncHoldTestSuite) TestWaitsAsync(t *gotest.T, done func()) {
	go func() {
		waiting.Done()
		waiting.Wait()
		mark("running")
		select {}
	}()
}

func (s *AsyncHoldTestSuite) TestFailsAsync(t *gotest.T, done func()) {
	failed := make(chan struct{})
	gotest.Go(t, func() {
		defer close(failed)
		gotest.Fail(t, "a helper goroutine fails")
	})
	go func() {
		<-failed
		time.Sleep(300 * time.Millisecond)
		waiting.Done()
		select {}
	}()
}
