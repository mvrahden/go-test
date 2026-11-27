package shop

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CartTestSuite struct{}

func (s *CartTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 10 * time.Second}
}

func (s *CartTestSuite) TestAdd(t *gotest.T) {
	t.When("both numbers are positive", func(w *gotest.T) {
		w.It("returns their sum", func(it *gotest.T) {
			gotest.Equal(it, 3, Add(1, 2))
			gotest.Equal(it, 5, Add(2, 3))
		})
		w.It("keeps the order irrelevant", func(it *gotest.T) {
			gotest.Equal(it, Add(2, 1), Add(1, 2))
		})
	})
	t.When("one number is negative", func(w *gotest.T) {
		w.It("returns the difference", func(it *gotest.T) {
			gotest.Equal(it, 1, Add(3, -2))
		})
	})
}

func (s *CartTestSuite) TestRemove(t *gotest.T) {
	gotest.Equal(t, 0, Add(1, -1))
}

func (s *CartTestSuite) TestEmpty(t *gotest.T) {
	gotest.Zero(t, Add(0, 0))
}

func (s *CartTestSuite) TestTotal(t *gotest.T) {
	gotest.Equal(t, 6, Add(Add(1, 2), 3))
}

func (s *CartTestSuite) TestRounds(t *gotest.T) {
	gotest.Equal(t, 4, Add(2, 2))
	gotest.NotZero(t, Add(2, 2))
}

func (s *CartTestSuite) TestCopies(t *gotest.T) {
	t.It("first copy", func(it *gotest.T) {
		gotest.Equal(it, 2, Add(1, 1))
	})
	t.It("second copy", func(it *gotest.T) {
		gotest.Equal(it, 2, Add(1, 1))
	})
}
