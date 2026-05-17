package inventory

import "github.com/mvrahden/go-test/pkg/gotest"

type InventoryTestSuite struct {
	stock *StockLevel
}

func (s *InventoryTestSuite) BeforeEach(t *gotest.T) {
	s.stock = &StockLevel{SKU: "SKU-001", Quantity: 100, Reserved: 0}
}

func (s *InventoryTestSuite) AfterEach(t *gotest.T) {
	s.stock = nil
}

func (s *InventoryTestSuite) TestReserveStock(t *gotest.T) {
	t.When("reserving within available quantity", func(t *gotest.T) {
		ok := s.stock.Reserve(30)

		t.It("succeeds", func(t *gotest.T) {
			gotest.True(t, ok)
		})
		t.It("reduces the available quantity", func(t *gotest.T) {
			gotest.Equal(t, 70, s.stock.Available())
		})
		t.It("reports less available than total", func(t *gotest.T) {
			gotest.Less(t, s.stock.Available(), s.stock.Quantity)
		})
		t.It("differs from the original available", func(t *gotest.T) {
			gotest.NotEqual(t, 100, s.stock.Available())
		})
	})
}

func (s *InventoryTestSuite) TestReserveExceedsAvailable(t *gotest.T) {
	t.When("reserving more than available", func(t *gotest.T) {
		s.stock.Reserve(80)
		ok := s.stock.Reserve(30)

		t.It("fails", func(t *gotest.T) {
			gotest.False(t, ok)
		})
		t.It("does not change the reserved count", func(t *gotest.T) {
			gotest.Equal(t, 80, s.stock.Reserved)
		})
		t.It("keeps available non-negative", func(t *gotest.T) {
			gotest.GreaterOrEqual(t, s.stock.Available(), 0)
		})
	})
}

func (s *InventoryTestSuite) TestRestock(t *gotest.T) {
	t.When("restocking an item with existing reservations", func(t *gotest.T) {
		s.stock.Reserve(50)
		s.stock.Restock(25)

		t.It("increases the total quantity", func(t *gotest.T) {
			gotest.Greater(t, s.stock.Quantity, 100)
		})
		t.It("keeps reservations unchanged", func(t *gotest.T) {
			gotest.Equal(t, 50, s.stock.Reserved)
		})
		t.It("has available at most the total quantity", func(t *gotest.T) {
			gotest.LessOrEqual(t, s.stock.Available(), s.stock.Quantity)
		})
	})
}
