package calc

import "github.com/mvrahden/go-test/pkg/gotest"

type CalcTestSuite struct{}

func (s *CalcTestSuite) TestSub(t *gotest.T) {
	t.T().Skipf("broken since %s", "Monday")
	gotest.Equal(t, 1, Sub(3, 2))
}

func (s *CalcTestSuite) TestSubOnce(t *gotest.T) {
	t.T().Skip("flaky")
	gotest.Equal(t, 0, Sub(2, 2))
	gotest.Equal(t, 0, Sub(3, 3))
}

func (s *CalcTestSuite) TestSubNever(t *gotest.T) {
	if Sub(2, 3) > 0 {
		t.T().SkipNow()
	}
	t.T().Skip("later")
	gotest.Equal(t, -1, Sub(2, 3))
}

func (s *CalcTestSuite) TestSubLater(t *gotest.T) {
	t.Skipf("until %s", "later")
	gotest.Equal(t, 2, Sub(4, 2))
}
