package cart

import "github.com/mvrahden/go-test/pkg/gotest"

type ShoppingCartTestSuite struct {
	cart    *cart
	catalog map[string]float64
}

func (s *ShoppingCartTestSuite) BeforeEach(t *gotest.T) {
	s.catalog = map[string]float64{"Apple": 1.50, "Bread": 3.00, "Milk": 2.50}
	s.cart = newCart(s.catalog)
}

func (s *ShoppingCartTestSuite) TestAddSingleItem(t *gotest.T) {
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
}

func (s *ShoppingCartTestSuite) TestAddMultipleItems(t *gotest.T) {
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
}

func (s *ShoppingCartTestSuite) TestAddSameItemTwice(t *gotest.T) {
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

func (s *ShoppingCartTestSuite) TestReduceQuantity(t *gotest.T) {
	t.When("reducing quantity below current amount", func(t *gotest.T) {
		s.cart.Add("Apple", 3)
		s.cart.Remove("Apple", 1)

		t.It("decreases without removing the item", func(t *gotest.T) {
			gotest.Equal(t, 2, s.cart.Quantity("Apple"))
		})
		t.It("keeps the item in the cart", func(t *gotest.T) {
			gotest.Contains(t, s.cart.Items(), "Apple")
		})
	})
}

func (s *ShoppingCartTestSuite) TestRemoveAllOfItem(t *gotest.T) {
	t.When("removing all of an item", func(t *gotest.T) {
		s.cart.Add("Apple", 3)
		s.cart.Remove("Apple", 3)

		t.It("removes the item entirely", func(t *gotest.T) {
			gotest.Empty(t, s.cart.Items())
		})
	})
}

func (s *ShoppingCartTestSuite) TestApplyDiscount(t *gotest.T) {
	t.When("applying a 10 percent discount", func(t *gotest.T) {
		s.cart.Add("Bread", 2)
		s.cart.ApplyDiscount(0.10)

		t.It("reduces the total within tolerance", func(t *gotest.T) {
			gotest.InDelta(t, 5.40, s.cart.Total(), 0.01)
		})
	})
}

func (s *ShoppingCartTestSuite) TestApplyExcessiveDiscount(t *gotest.T) {
	t.When("the discount exceeds 100 percent", func(t *gotest.T) {
		s.cart.Add("Bread", 2)
		s.cart.ApplyDiscount(1.5)

		t.It("clamps to zero", func(t *gotest.T) {
			gotest.GreaterOrEqual(t, s.cart.Total(), 0.0)
			gotest.Equal(t, 0.0, s.cart.Total())
		})
	})
}

func (s *ShoppingCartTestSuite) TestCheckoutEmpty(t *gotest.T) {
	t.When("the cart is empty", func(t *gotest.T) {
		err := s.cart.Checkout()

		t.It("returns an error", func(t *gotest.T) {
			gotest.ErrorIs(t, err, ErrEmptyCart)
		})
	})
}

func (s *ShoppingCartTestSuite) TestCheckoutWithItems(t *gotest.T) {
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
