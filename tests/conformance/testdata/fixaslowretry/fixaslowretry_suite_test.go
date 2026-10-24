package fixaslowretry

import "github.com/mvrahden/go-test/pkg/gotest"

// SlowTestSuite reads the fixture its second attempt brought up.
type SlowTestSuite struct {
	Slow *SlowSharedFixture
}

func (s *SlowTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "up", s.Slow.State) }
