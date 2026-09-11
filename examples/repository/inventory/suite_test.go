package inventory

import "github.com/mvrahden/go-test/pkg/gotest"

type inventoryCtx struct {
	stock *StockLevel
}

type InventoryTestSuite struct{}

func (s *InventoryTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *InventoryTestSuite) BeforeEach(t *gotest.T) *inventoryCtx {
	return &inventoryCtx{stock: &StockLevel{SKU: "SKU-001", Quantity: 100, Reserved: 0}}
}

func (s *InventoryTestSuite) AfterEach(t *gotest.T, c *inventoryCtx) {
	c.stock = nil
}

func (s *InventoryTestSuite) TestReserveStock(t *gotest.T, c *inventoryCtx) {
	t.When("reserving within available quantity", func(t *gotest.T) {
		ok := c.stock.Reserve(30)

		t.It("succeeds", func(t *gotest.T) {
			gotest.True(t, ok)
		})
		t.It("reduces the available quantity", func(t *gotest.T) {
			gotest.Equal(t, 70, c.stock.Available())
		})
		t.It("reports less available than total", func(t *gotest.T) {
			gotest.Less(t, c.stock.Available(), c.stock.Quantity)
		})
		t.It("differs from the original available", func(t *gotest.T) {
			gotest.NotEqual(t, 100, c.stock.Available())
		})
	})
}

func (s *InventoryTestSuite) TestReserveExceedsAvailable(t *gotest.T, c *inventoryCtx) {
	t.When("reserving more than available", func(t *gotest.T) {
		c.stock.Reserve(80)
		ok := c.stock.Reserve(30)

		t.It("fails", func(t *gotest.T) {
			gotest.False(t, ok)
		})
		t.It("does not change the reserved count", func(t *gotest.T) {
			gotest.Equal(t, 80, c.stock.Reserved)
		})
		t.It("keeps available non-negative", func(t *gotest.T) {
			gotest.GreaterOrEqual(t, c.stock.Available(), 0)
		})
	})
}

func (s *InventoryTestSuite) TestRestock(t *gotest.T, c *inventoryCtx) {
	t.When("restocking an item with existing reservations", func(t *gotest.T) {
		c.stock.Reserve(50)
		c.stock.Restock(25)

		t.It("increases the total quantity", func(t *gotest.T) {
			gotest.Greater(t, c.stock.Quantity, 100)
		})
		t.It("keeps reservations unchanged", func(t *gotest.T) {
			gotest.Equal(t, 50, c.stock.Reserved)
		})
		t.It("has available at most the total quantity", func(t *gotest.T) {
			gotest.LessOrEqual(t, c.stock.Available(), c.stock.Quantity)
		})
	})
}
