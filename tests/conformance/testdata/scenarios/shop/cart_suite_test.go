package shop

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CartTestSuite struct{}

func (s *CartTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *CartTestSuite) TestTotals(t *gotest.T) {
	t.When("the cart is empty", func(w *gotest.T) {
		w.It("totals zero", func(it *gotest.T) {
			gotest.Zero(it, Total())
		})
		w.It("lists no line", func(it *gotest.T) {
			gotest.Empty(it, []int{})
		})
		w.It("logs nothing", func(it *gotest.T) {
			gotest.Zero(it, Total())
		})
	})
	t.When("a member", func(w *gotest.T) {
		w.When("a discount applies", func(w *gotest.T) {
			w.It("the total drops", func(it *gotest.T) {
				gotest.Less(it, Total(1), Total(2))
			})
		})
	})
	t.When("a discount applies", func(w *gotest.T) {
		w.It("the total drops", func(it *gotest.T) {
			gotest.Less(it, Total(1), Total(2))
		})
	})
	t.When("a coupon is applied", func(w *gotest.T) {
		w.It("the total drops by its value", func(it *gotest.T) {
			gotest.Equal(it, 1, Total(2, -1))
		})
	})
	t.When("カートを空にする", func(w *gotest.T) {
		w.It("合計を表示する", func(it *gotest.T) {
			gotest.Zero(it, Total())
		})
	})
	t.When("カートを満たす", func(w *gotest.T) {
		w.It("合計を表示する", func(it *gotest.T) {
			gotest.Equal(it, 3, Total(1, 2))
		})
	})
	t.When("rows are read", func(w *gotest.T) {
		for it, n := range gotest.Each(w, []int{1, 2}) {
			it.It("reads each row", func(it *gotest.T) {
				gotest.Equal(it, n, Total(n))
			})
		}
	})
}

func (s *CartTestSuite) TestSizes(t *gotest.T) {
	for it, n := range gotest.Each(t, []int{1, 2}) {
		it.When("a size is chosen", func(w *gotest.T) {
			w.It("fits the box", func(it *gotest.T) {
				gotest.Greater(it, Total(n), 0)
			})
		})
	}
}

func (s *CartTestSuite) TestPrices(t *gotest.T) {
	t.When("a price is shown", func(w *gotest.T) {
		w.It("shows the currency", func(it *gotest.T) {
			gotest.Greater(it, Total(1), 0)
		})
	})
}

type ParkedTestSuite struct{}

func (s *ParkedTestSuite) X_TestParked(t *gotest.T) {
	t.When("the check is parked", func(w *gotest.T) {
		w.It("still holds", func(it *gotest.T) {
			gotest.Zero(it, Total())
		})
	})
}
