package slowsetup

import "github.com/mvrahden/go-test/pkg/gotest"

// ContainerTestSuite reads the slow fixture's state.
type ContainerTestSuite struct {
	Container *ContainerSharedFixture
}

func (s *ContainerTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "up", s.Container.State) }
