package fixastoprelease

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// QuickTestSuite is the slow fixture's only reader.
type QuickTestSuite struct {
	Slow *SlowSharedFixture
}

func (s *QuickTestSuite) TestReadsSlow(t *gotest.T) { gotest.Equal(t, "up", s.Slow.State) }

// LongTestSuite holds its fixture until the run is stopped.
type LongTestSuite struct {
	Held *HeldSharedFixture
}

func (s *LongTestSuite) TestHolds(t *gotest.T) {
	gotest.Equal(t, "up", s.Held.State)
	select {
	case <-t.Context().Done():
	case <-time.After(20 * time.Second):
	}
}
