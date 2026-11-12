package sharedembedhyd

import "github.com/mvrahden/go-test/pkg/gotest"

type StateTestSuite struct{ State *StateSharedFixture }

func (s *StateTestSuite) TestState(t *gotest.T) {
	gotest.Equal(t, "from mem://state", s.State.Label)
	gotest.Equal(t, "mem", s.State.Host)
}
