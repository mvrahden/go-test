package calc

import (
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CalcTestSuite struct{}

func (s *CalcTestSuite) BeforeAll(t *gotest.T) {
	gotest.NoError(t, errors.New("the database is down"))
}

func (s *CalcTestSuite) AfterAll(t *gotest.T) {}

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

func (s *CalcTestSuite) TestSubAgain(t *gotest.T) {
	gotest.Equal(t, 2, Sub(4, 2))
}
