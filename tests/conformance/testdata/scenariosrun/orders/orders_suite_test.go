package orders

import (
	"errors"
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type OrdersTestSuite struct{}

func (s *OrdersTestSuite) BeforeAll(t *gotest.T) {
	if os.Getenv("SCENRUNMOD_SETUP") == "fail" {
		gotest.NoError(t, errors.New("the database is down"))
	}
}

func (s *OrdersTestSuite) AfterAll(t *gotest.T) {}

func (s *OrdersTestSuite) TestPlacing(t *gotest.T) {
	t.When("the order is placed", func(w *gotest.T) {
		w.It("charges the card", func(it *gotest.T) {
			gotest.Equal(it, 2, 1+1)
		})
		w.It("sends the receipt", func(it *gotest.T) {
			gotest.Equal(it, 4, 2+2)
		})
	})
}
