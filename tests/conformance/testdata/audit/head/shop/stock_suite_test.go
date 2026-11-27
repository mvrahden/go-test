package shop_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"

	"auditmod/shop"
)

type StockTestSuite struct{}

func (s *StockTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: gotest.NoDeadline}
}

func (s *StockTestSuite) SuiteGuard() string { return "" }

func (s *StockTestSuite) TestCount(t *gotest.T) {
	t.When("the stock is full", func(w *gotest.T) {
		w.It("reports the maximum", func(it *gotest.T) {
			gotest.Equal(it, 9, shop.Add(4, 5))
		})
	})
	t.When("the stock is refilled", func(w *gotest.T) {
		w.It("counts every item", func(it *gotest.T) {
			gotest.Equal(it, 10, shop.Add(5, 5))
		})
	})
	t.When("the stock is empty", func(w *gotest.T) {
		w.It("reports zero", func(it *gotest.T) {
			gotest.Zero(it, shop.Add(0, 0))
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

// TestLegacy stays a stdlib test.
//
//nolint:stdlib-test // kept for go test
func TestLegacy(t *testing.T) {
	if shop.Add(1, 1) != 2 {
		t.Fatal("1 + 1")
	}
}

//nolint:stdlib-test // a second stdlib test
func TestLegacyTwice(t *testing.T) {
	if shop.Add(2, 2) != 4 {
		t.Fatal("2 + 2")
	}
}
