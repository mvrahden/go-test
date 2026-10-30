package sharedteardowntext

import "github.com/mvrahden/go-test/pkg/gotest"

type WordyTestSuite struct {
	Wordy *WordySharedFixture
}

func (s *WordyTestSuite) TestUp(t *gotest.T) { gotest.Equal(t, "up", s.Wordy.State) }
