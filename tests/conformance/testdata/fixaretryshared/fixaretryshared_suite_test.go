package fixaretryshared

import "github.com/mvrahden/go-test/pkg/gotest"

// RetryTestSuite reads the fixture its second attempt brought up.
type RetryTestSuite struct {
	Retry *RetrySharedFixture
}

func (s *RetryTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "up", s.Retry.State) }
