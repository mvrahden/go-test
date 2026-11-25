package checkout

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PaymentTestSuite struct{}

func (s *PaymentTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *PaymentTestSuite) TestPayment(t *gotest.T) {
	t.When("an order is paid", func(w *gotest.T) {
		w.It("marks it paid", func(it *gotest.T) {
			gotest.True(it, Paid(2, 2))
		})
	})
	t.When("a payment fails", func(w *gotest.T) {
		w.It("keeps the order open", func(it *gotest.T) {
			gotest.True(it, Paid(1, 2), "the order is left open")
		})
	})
	t.When("the gateway is down", func(w *gotest.T) {
		w.It("retries later", func(it *gotest.T) {
			it.T().Skip("the gateway is not reachable here")
		})
	})
	t.When("a receipt is printed", func(w *gotest.T) {
		w.It("prints the total", func(it *gotest.T) {
			_ = Paid(1, 1)
		})
	})
}

func (s *PaymentTestSuite) TestParcels(t *gotest.T) {
	for it, ok := range gotest.Each(t, []bool{true, false}) {
		it.When("a parcel is weighed", func(w *gotest.T) {
			w.It("prices it by weight", func(it *gotest.T) {
				gotest.True(it, ok, "the second parcel is mispriced")
			})
		})
	}
}

type BankTestSuite struct{}

func (s *BankTestSuite) SuiteGuard() string { return "the bank is offline here" }

func (s *BankTestSuite) TestQueue(t *gotest.T) {
	t.When("the bank is offline", func(w *gotest.T) {
		w.It("queues the order", func(it *gotest.T) {
			gotest.True(it, Paid(1, 1))
		})
	})
}
