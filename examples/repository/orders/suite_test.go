package orders

import "github.com/mvrahden/go-test/pkg/gotest"

type OrderRepositoryTestSuite struct {
	store *orderStore
}

func (s *OrderRepositoryTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.FailFast = true
	return cfg
}

func (s *OrderRepositoryTestSuite) BeforeAll(t *gotest.T) {
	s.store = newOrderStore()
}

func (s *OrderRepositoryTestSuite) TestPlaceOrder(t *gotest.T) {
	t.When("stock is sufficient", func(t *gotest.T) {
		order, err := s.store.Place("ord-1", "Widget", 5, 10)

		t.It("succeeds without error", func(t *gotest.T) {
			gotest.NoError(t, err)
		})
		t.It("creates a pending order", func(t *gotest.T) {
			gotest.Equal(t, OrderPending, order.Status)
		})
		t.It("records the quantity", func(t *gotest.T) {
			gotest.Equal(t, 5, order.Quantity)
		})
	})

	t.When("stock is insufficient", func(t *gotest.T) {
		_, err := s.store.Place("ord-2", "Widget", 15, 10)

		t.It("returns an error", func(t *gotest.T) {
			gotest.Error(t, err)
		})
		t.It("is specifically ErrInsufficientStock", func(t *gotest.T) {
			gotest.ErrorIs(t, err, ErrInsufficientStock)
		})
	})
}

func (s *OrderRepositoryTestSuite) TestOrderLifecycle(t *gotest.T) {
	t.When("an order progresses through its lifecycle", func(t *gotest.T) {
		order, _ := s.store.Place("ord-3", "Gadget", 1, 5)

		t.It("starts as pending", func(t *gotest.T) {
			gotest.Equal(t, OrderPending, order.Status)
		})
		t.It("transitions to confirmed", func(t *gotest.T) {
			ok := s.store.Confirm("ord-3")
			gotest.True(t, ok)
			gotest.Equal(t, OrderConfirmed, order.Status)
		})
		t.It("transitions to shipped", func(t *gotest.T) {
			ok := s.store.Ship("ord-3")
			gotest.True(t, ok)
			gotest.Equal(t, OrderShipped, order.Status)
		})
	})
}
