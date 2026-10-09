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
	s.Pool.Acquire()
	defer s.Pool.Release()
	logLine("running")
	select {
	case <-t.Context().Done():
	case <-time.After(time.Minute):
	}
}
