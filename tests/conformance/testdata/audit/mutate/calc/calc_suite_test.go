package calc_test

import (
	"os"

	"mutmod/calc"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CalcTestSuite tests Discount weakly: never at 100.
type CalcTestSuite struct{}

func (s *CalcTestSuite) TestDiscount(t *gotest.T) {
	gotest.Equal(t, 20, calc.Discount(200))
	gotest.Equal(t, 0, calc.Discount(50))
}

func (s *CalcTestSuite) TestSum(t *gotest.T) {
	gotest.Equal(t, 6, calc.Sum(3))
}

func (s *CalcTestSuite) TestHalf(t *gotest.T) {
	gotest.Equal(t, 2, calc.Half(4))
	gotest.NotEqual(t, "1", os.Getenv("MUTMOD_FAIL"), "MUTMOD_FAIL fails the suite")
}
