package cart_test

import "github.com/mvrahden/go-test/pkg/gotest"

type cartCtx struct {
	items map[string]int
}

type ShoppingCartTestSuite struct{}

func (s *ShoppingCartTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ShoppingCartTestSuite) BeforeEach(t *gotest.T) *cartCtx {
	return &cartCtx{items: map[string]int{}}
}

func (s *ShoppingCartTestSuite) TestAddItem(t *gotest.T, c *cartCtx) {
	t.When("adding items to a fresh cart", func(t *gotest.T) {
		c.items["Laptop"] = 1
		c.items["Mouse"] = 2

		t.It("tracks each item", func(t *gotest.T) {
			gotest.Len(t, c.items, 2)
		})
		t.It("stores the correct quantity", func(t *gotest.T) {
			gotest.Equal(t, 2, c.items["Mouse"])
		})
	})
}

func (s *ShoppingCartTestSuite) TestRemoveItem(t *gotest.T, c *cartCtx) {
	c.items["Phone"] = 1

	t.When("removing the last item", func(t *gotest.T) {
		delete(c.items, "Phone")

		t.It("leaves the cart empty", func(t *gotest.T) {
			gotest.Empty(t, c.items)
		})
	})
}
