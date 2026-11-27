package calc

import "github.com/mvrahden/go-test/pkg/gotest"

type CalcTestSuite struct{}

func (s *CalcTestSuite) TestSub(t *gotest.T) {
	gotest.Equal(t, 1, Sub(3, 2))
}

func (s *CalcTestSuite) TestSubOnce(t *gotest.T) {
	t.T().Skip("flaky")
	gotest.Equal(t, 0, Sub(2, 2))
}

func (s *CalcTestSuite) TestSubNever(t *gotest.T) {
	gotest.Equal(t, -1, Sub(2, 3))
}
