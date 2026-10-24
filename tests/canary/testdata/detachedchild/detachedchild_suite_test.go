package detachedchild

import "github.com/mvrahden/go-test/pkg/gotest"

// DetachTestSuite reads the fixture's state.
type DetachTestSuite struct {
	Detach *DetachSharedFixture
}

func (s *DetachTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "up", s.Detach.State) }
