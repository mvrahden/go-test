package orders

import (
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// --- Domain types ---

var ErrInsufficientStock = errors.New("insufficient stock")

type OrderStatus int

const (
	OrderPending OrderStatus = iota
	OrderConfirmed
	OrderShipped
)

type Order struct {
	ID       string
	Item     string
	Quantity int
	Status   OrderStatus
}

type orderStore struct {
	orders map[string]*Order
}

func newOrderStore() *orderStore {
	return &orderStore{orders: make(map[string]*Order)}
}

func (s *orderStore) Place(id, item string, qty, stock int) (*Order, error) {
	if qty > stock {
		return nil, ErrInsufficientStock
	}
	o := &Order{ID: id, Item: item, Quantity: qty, Status: OrderPending}
	s.orders[id] = o
	return o, nil
}

func (s *orderStore) Confirm(id string) bool {
	if o, ok := s.orders[id]; ok && o.Status == OrderPending {
		o.Status = OrderConfirmed
		return true
	}
	return false
}

func (s *orderStore) Ship(id string) bool {
	if o, ok := s.orders[id]; ok && o.Status == OrderConfirmed {
		o.Status = OrderShipped
		return true
	}
	return false
}

func (s *orderStore) Get(id string) (*Order, bool) {
	o, ok := s.orders[id]
	return o, ok
}

// --- Test Suite ---

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
