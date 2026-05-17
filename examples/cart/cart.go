package cart

import "errors"

var ErrEmptyCart = errors.New("cart is empty")

type item struct {
	Name     string
	Price    float64
	Quantity int
}

type cart struct {
	items    map[string]*item
	catalog  map[string]float64
	discount float64
}

func newCart(catalog map[string]float64) *cart {
	return &cart{items: make(map[string]*item), catalog: catalog}
}

func (c *cart) Add(name string, qty int) {
	if it, ok := c.items[name]; ok {
		it.Quantity += qty
		return
	}
	c.items[name] = &item{Name: name, Price: c.catalog[name], Quantity: qty}
}

func (c *cart) Remove(name string, qty int) {
	it, ok := c.items[name]
	if !ok {
		return
	}
	it.Quantity -= qty
	if it.Quantity <= 0 {
		delete(c.items, name)
	}
}

func (c *cart) UniqueItems() int { return len(c.items) }

func (c *cart) Items() []string {
	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	return keys
}

func (c *cart) Quantity(name string) int {
	if it, ok := c.items[name]; ok {
		return it.Quantity
	}
	return 0
}

func (c *cart) Total() float64 {
	var sum float64
	for _, it := range c.items {
		sum += it.Price * float64(it.Quantity)
	}
	return sum * (1 - c.discount)
}

func (c *cart) ApplyDiscount(pct float64) {
	c.discount = pct
	if c.discount > 1 {
		c.discount = 1
	}
}

func (c *cart) Checkout() error {
	if len(c.items) == 0 {
		return ErrEmptyCart
	}
	c.items = make(map[string]*item)
	return nil
}
