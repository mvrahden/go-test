package shop

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StockTestSuite struct{}

func (s *StockTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *StockTestSuite) TestShelf(t *gotest.T) {
	t.When("stock is counted", func(w *gotest.T) {
		w.It("counts each item", func(it *gotest.T) {
			gotest.Equal(it, 2, Total(1, 1))
		})
	})
}

func (s *StockTestSuite) TestStore(t *gotest.T) {
	t.When("stock is counted", func(w *gotest.T) {
		w.It("counts each item", func(it *gotest.T) {
			gotest.Equal(it, 3, Total(1, 2))
		})
	})
}

func (s *StockTestSuite) TestGuests(t *gotest.T) {
	t.When("a guest", func(w *gotest.T) {
		w.When("a discount applies", func(w *gotest.T) {
			w.It("the total drops", func(it *gotest.T) {
				gotest.Less(it, Total(1), Total(3))
			})
		})
	})
}
