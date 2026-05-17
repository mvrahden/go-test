package cart

import (
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

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

func (c *cart) UniqueItems() int    { return len(c.items) }
func (c *cart) Items() []string     { keys := make([]string, 0, len(c.items)); for k := range c.items { keys = append(keys, k) }; return keys }
func (c *cart) Quantity(name string) int { if it, ok := c.items[name]; ok { return it.Quantity }; return 0 }

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

// --- Test Suite ---

type ShoppingCartTestSuite struct {
	cart    *cart
	catalog map[string]float64
}

func (s *ShoppingCartTestSuite) BeforeEach(t *gotest.T) {
	s.catalog = map[string]float64{"Apple": 1.50, "Bread": 3.00, "Milk": 2.50}
	s.cart = newCart(s.catalog)
}

func (s *ShoppingCartTestSuite) TestAddItem(t *gotest.T) {
	t.When("adding a single item", func(t *gotest.T) {
		s.cart.Add("Apple", 2)

		t.It("increases the item count", func(t *gotest.T) {
			gotest.Equal(t, 1, s.cart.UniqueItems())
		})
		t.It("tracks the quantity", func(t *gotest.T) {
			gotest.Equal(t, 2, s.cart.Quantity("Apple"))
		})
		t.It("calculates the line total", func(t *gotest.T) {
			gotest.Equal(t, 3.00, s.cart.Total())
		})
	})

	t.When("adding multiple different items", func(t *gotest.T) {
		s.cart.Add("Apple", 1)
		s.cart.Add("Bread", 1)

		t.It("counts all unique items", func(t *gotest.T) {
			gotest.Equal(t, 2, s.cart.UniqueItems())
		})
		t.It("sums the total across items", func(t *gotest.T) {
			gotest.Equal(t, 4.50, s.cart.Total())
		})
	})

	t.When("adding the same item twice", func(t *gotest.T) {
		s.cart.Add("Milk", 1)
		s.cart.Add("Milk", 2)

		t.It("merges the quantities", func(t *gotest.T) {
			gotest.Equal(t, 3, s.cart.Quantity("Milk"))
		})
		t.It("keeps only one unique entry", func(t *gotest.T) {
			gotest.Equal(t, 1, s.cart.UniqueItems())
		})
	})
}

func (s *ShoppingCartTestSuite) TestRemoveItem(t *gotest.T) {
	s.cart.Add("Apple", 3)

	t.When("reducing quantity below current amount", func(t *gotest.T) {
		s.cart.Remove("Apple", 1)

		t.It("decreases without removing the item", func(t *gotest.T) {
			gotest.Equal(t, 2, s.cart.Quantity("Apple"))
		})
		t.It("keeps the item in the cart", func(t *gotest.T) {
			gotest.Contains(t, s.cart.Items(), "Apple")
		})
	})

	t.When("removing all of an item", func(t *gotest.T) {
		s.cart.Remove("Apple", 3)

		t.It("removes the item entirely", func(t *gotest.T) {
			gotest.Empty(t, s.cart.Items())
		})
	})
}

func (s *ShoppingCartTestSuite) TestApplyDiscount(t *gotest.T) {
	s.cart.Add("Bread", 2)

	t.When("applying a 10 percent discount", func(t *gotest.T) {
		s.cart.ApplyDiscount(0.10)

		t.It("reduces the total within tolerance", func(t *gotest.T) {
			gotest.InDelta(t, 5.40, s.cart.Total(), 0.01)
		})
	})

	t.When("the discount exceeds 100 percent", func(t *gotest.T) {
		s.cart.ApplyDiscount(1.5)

		t.It("clamps to zero", func(t *gotest.T) {
			gotest.GreaterOrEqual(t, s.cart.Total(), 0.0)
			gotest.Equal(t, 0.0, s.cart.Total())
		})
	})
}

func (s *ShoppingCartTestSuite) TestCheckout(t *gotest.T) {
	t.When("the cart is empty", func(t *gotest.T) {
		err := s.cart.Checkout()

		t.It("returns an error", func(t *gotest.T) {
			gotest.ErrorIs(t, err, ErrEmptyCart)
		})
	})

	t.When("the cart has items", func(t *gotest.T) {
		s.cart.Add("Apple", 2)
		s.cart.Add("Milk", 1)
		err := s.cart.Checkout()

		t.It("succeeds without error", func(t *gotest.T) {
			gotest.NoError(t, err)
		})
		t.It("clears the cart", func(t *gotest.T) {
			gotest.Empty(t, s.cart.Items())
		})
	})
}
