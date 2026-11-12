package sharedembedunexp

import "github.com/mvrahden/go-test/pkg/gotest"

type StateTestSuite struct{ State *StateSharedFixture }

func (s *StateTestSuite) TestAddr(t *gotest.T) { gotest.Equal(t, "mem://state", s.State.Addr) }
