package shop

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CartTestSuite struct{}

func (s *CartTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *CartTestSuite) TestAdd(t *gotest.T) {
	t.When("both numbers are positive", func(w *gotest.T) {
		w.It("returns their sum", func(it *gotest.T) {
			gotest.Equal(it, 3, Add(1, 2))
			gotest.Equal(it.Soft(), 5, Add(2, 3))
		})
		w.It("ignores the order", func(it *gotest.T) {
			// Laid out and commented anew; the tokens are the same.
			gotest.Equal(it,
				Add(2, 1),
				Add(1, 2))
		})
	})
	t.When("one number is negative", func(w *gotest.T) {
		w.It("returns the difference", func(it *gotest.T) {
			gotest.Equal(it, 2, Add(4, -2))
		})
	})
}

func (s *CartTestSuite) X_TestEmpty(t *gotest.T) {
	gotest.Zero(t, Add(0, 0))
}

func (s *CartTestSuite) F_TestTotal(t *gotest.T) {
	gotest.Equal(t, 6, Add(Add(1, 2), 3))
}

func (s *CartTestSuite) TestRounds(t *gotest.T) {
	t.AssertsBy("NotZero checks the sum")
	gotest.NotZero(t, Add(2, 2))
}

func (s *CartTestSuite) TestCopies(t *gotest.T) {
	t.It("the only copy", func(it *gotest.T) {
		gotest.Equal(it, 2, Add(1, 1))
	})
}
