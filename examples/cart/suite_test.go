package cart

import "github.com/mvrahden/go-test/pkg/gotest"

type cartCtx struct {
	cart *cart
}

type ShoppingCartTestSuite struct{}

func (s *ShoppingCartTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ShoppingCartTestSuite) BeforeEach(t *gotest.T) *cartCtx {
	catalog := map[string]float64{"Apple": 1.50, "Bread": 3.00, "Milk": 2.50}
	return &cartCtx{cart: newCart(catalog)}
}

func (s *ShoppingCartTestSuite) TestAddSingleItem(t *gotest.T, c *cartCtx) {
	t.When("adding a single item", func(t *gotest.T) {
		c.cart.Add("Apple", 2)

		t.It("increases the item count", func(t *gotest.T) {
			gotest.Equal(t, 1, c.cart.UniqueItems())
		})
		t.It("tracks the quantity", func(t *gotest.T) {
			gotest.Equal(t, 2, c.cart.Quantity("Apple"))
		})
		t.It("calculates the line total", func(t *gotest.T) {
			gotest.Equal(t, 3.00, c.cart.Total())
		})
	})
}

func (s *ShoppingCartTestSuite) TestAddMultipleItems(t *gotest.T, c *cartCtx) {
	t.When("adding multiple different items", func(t *gotest.T) {
		c.cart.Add("Apple", 1)
		c.cart.Add("Bread", 1)

		t.It("counts all unique items", func(t *gotest.T) {
			gotest.Equal(t, 2, c.cart.UniqueItems())
		})
		t.It("sums the total across items", func(t *gotest.T) {
			gotest.Equal(t, 4.50, c.cart.Total())
		})
	})
}

func (s *ShoppingCartTestSuite) TestAddSameItemTwice(t *gotest.T, c *cartCtx) {
	t.When("adding the same item twice", func(t *gotest.T) {
		c.cart.Add("Milk", 1)
		c.cart.Add("Milk", 2)

		t.It("merges the quantities", func(t *gotest.T) {
			gotest.Equal(t, 3, c.cart.Quantity("Milk"))
		})
		t.It("keeps only one unique entry", func(t *gotest.T) {
			gotest.Equal(t, 1, c.cart.UniqueItems())
		})
	})
}

func (s *ShoppingCartTestSuite) TestReduceQuantity(t *gotest.T, c *cartCtx) {
	t.When("reducing quantity below current amount", func(t *gotest.T) {
		c.cart.Add("Apple", 3)
		c.cart.Remove("Apple", 1)

		t.It("decreases without removing the item", func(t *gotest.T) {
			gotest.Equal(t, 2, c.cart.Quantity("Apple"))
		})
		t.It("keeps the item in the cart", func(t *gotest.T) {
			gotest.Contains(t, c.cart.Items(), "Apple")
		})
	})
}

func (s *ShoppingCartTestSuite) TestRemoveAllOfItem(t *gotest.T, c *cartCtx) {
	t.When("removing all of an item", func(t *gotest.T) {
		c.cart.Add("Apple", 3)
		c.cart.Remove("Apple", 3)

		t.It("removes the item entirely", func(t *gotest.T) {
			gotest.Empty(t, c.cart.Items())
		})
	})
}

func (s *ShoppingCartTestSuite) TestApplyDiscount(t *gotest.T, c *cartCtx) {
	t.When("applying a 10 percent discount", func(t *gotest.T) {
		c.cart.Add("Bread", 2)
		c.cart.ApplyDiscount(0.10)

		t.It("reduces the total within tolerance", func(t *gotest.T) {
			gotest.InDelta(t, 5.40, c.cart.Total(), 0.01)
		})
	})
}

func (s *ShoppingCartTestSuite) TestApplyExcessiveDiscount(t *gotest.T, c *cartCtx) {
	t.When("the discount exceeds 100 percent", func(t *gotest.T) {
		c.cart.Add("Bread", 2)
		c.cart.ApplyDiscount(1.5)

		t.It("clamps to zero", func(t *gotest.T) {
			gotest.GreaterOrEqual(t, c.cart.Total(), 0.0)
			gotest.Equal(t, 0.0, c.cart.Total())
		})
	})
}

func (s *ShoppingCartTestSuite) TestCheckoutEmpty(t *gotest.T, c *cartCtx) {
	t.When("the cart is empty", func(t *gotest.T) {
		err := c.cart.Checkout()

		t.It("returns an error", func(t *gotest.T) {
			gotest.ErrorIs(t, err, ErrEmptyCart)
		})
	})
}

func (s *ShoppingCartTestSuite) TestCheckoutWithItems(t *gotest.T, c *cartCtx) {
	t.When("the cart has items", func(t *gotest.T) {
		c.cart.Add("Apple", 2)
		c.cart.Add("Milk", 1)
		err := c.cart.Checkout()

		t.It("succeeds without error", func(t *gotest.T) {
			gotest.NoError(t, err)
		})
		t.It("clears the cart", func(t *gotest.T) {
			gotest.Empty(t, c.cart.Items())
		})
	})
}
