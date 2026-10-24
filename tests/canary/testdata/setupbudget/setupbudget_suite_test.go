package setupbudget

import "github.com/mvrahden/go-test/pkg/gotest"

// SlowTestSuite reads a fixture that never comes up within its budget.
type SlowTestSuite struct {
	Slow *SlowSharedFixture
}

func (s *SlowTestSuite) TestState(t *gotest.T) { gotest.Equal(t, "late", s.Slow.State) }
