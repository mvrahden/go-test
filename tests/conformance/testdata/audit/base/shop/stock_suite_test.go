package shop_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"

	"auditmod/shop"
)

type StockTestSuite struct{}

func (s *StockTestSuite) TestCount(t *gotest.T) {
	t.When("the stock is full", func(w *gotest.T) {
		w.It("counts every item", func(it *gotest.T) {
			gotest.Equal(it, 10, shop.Add(5, 5))
		})
		w.It("reports the maximum", func(it *gotest.T) {
			gotest.Equal(it, 9, shop.Add(4, 5))
		})
	})
	for t, row := range gotest.Each(t, []struct {
		Desc    string
		a, b, n int
	}{
		{Desc: "small numbers", a: 1, b: 2, n: 3},
		{Desc: "large numbers", a: 100, b: 200, n: 300},
	}) {
		gotest.Equal(t, row.n, shop.Add(row.a, row.b))
	}
}

//nolint:stdlib-test // kept for go test
func TestLegacy(t *testing.T) {
	if shop.Add(1, 1) != 2 {
		t.Fatal("1 + 1")
	}
}
