package sharedembedpart

import "github.com/mvrahden/go-test/pkg/gotest"

type StateTestSuite struct{ State *StateSharedFixture }

func (s *StateTestSuite) TestHost(t *gotest.T) { gotest.Equal(t, "mem", s.State.Host) }
