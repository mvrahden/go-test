package orders

import "errors"

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
