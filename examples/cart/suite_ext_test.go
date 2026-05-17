package cart_test

import "github.com/mvrahden/go-test/pkg/gotest"

type ShoppingCartTestSuite struct {
	items map[string]int
}

func (s *ShoppingCartTestSuite) BeforeEach(t *gotest.T) {
	s.items = map[string]int{}
}

func (s *ShoppingCartTestSuite) TestAddItem(t *gotest.T) {
	t.When("adding items to a fresh cart", func(t *gotest.T) {
		s.items["Laptop"] = 1
		s.items["Mouse"] = 2

		t.It("tracks each item", func(t *gotest.T) {
			gotest.Len(t, s.items, 2)
		})
		t.It("stores the correct quantity", func(t *gotest.T) {
			gotest.Equal(t, 2, s.items["Mouse"])
		})
	})
}

func (s *ShoppingCartTestSuite) TestRemoveItem(t *gotest.T) {
	s.items["Phone"] = 1

	t.When("removing the last item", func(t *gotest.T) {
		delete(s.items, "Phone")

		t.It("leaves the cart empty", func(t *gotest.T) {
			gotest.Empty(t, s.items)
		})
	})
}
