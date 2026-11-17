package sharedembedok

import "github.com/mvrahden/go-test/pkg/gotest"

type StateTestSuite struct{ State *StateSharedFixture }

func (s *StateTestSuite) TestState(t *gotest.T) {
	gotest.Equal(t, "mem://state", s.State.Info.Addr)
	gotest.Equal(t, "from mem://state", s.State.Label)
}
