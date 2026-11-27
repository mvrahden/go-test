package stop_test

import "github.com/mvrahden/go-test/pkg/gotest"

// StopTestSuite is the suite an agent's turn weakens.
type StopTestSuite struct{}

func (s *StopTestSuite) TestAdds(t *gotest.T) {
	gotest.Equal(t, 2, 1+1)
}

func (s *StopTestSuite) TestSubtracts(t *gotest.T) {
	gotest.Equal(t, 0, 1-1)
}

func (s *StopTestSuite) TestMultiplies(t *gotest.T) {
	gotest.Equal(t, 4, 2*2)
}
