package hold

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldingTestSuite holds a pooled connection until it is told to stop.
type HoldingTestSuite struct {
	Pool *PoolFixture
}

func (s *HoldingTestSuite) TestHolds(t *gotest.T) {
	if !on("STOPPING_HOLD") {
		return
	}
	if on("STOPPING_IGNORE_CONTEXT") {
		// A method that never looks at its context cannot be stopped.
		logLine("running")
		time.Sleep(time.Minute)
		return
	}
	s.Pool.Acquire()
	defer s.Pool.Release()
	logLine("running")
	select {
	case <-t.Context().Done():
	case <-time.After(time.Minute):
	}
}

// LaterTestSuite is declared after HoldingTestSuite, so it would run next.
type LaterTestSuite struct{}

func (s *LaterTestSuite) TestLater(t *gotest.T) { logLine("later ran") }
