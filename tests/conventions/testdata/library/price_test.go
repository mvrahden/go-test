package price

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestTotal(t *testing.T) {
	gt := gotest.NewT(t)

	gt.When("the cart is empty", func(w *gotest.T) {
		w.It("totals zero", func(it *gotest.T) {
			gotest.Equal(it, 0, Total(nil))
		})
	})

	for it, tc := range gotest.Each(gt, []struct {
		Desc  string
		Items []int
		Want  int
	}{
		{Desc: "one item", Items: []int{5}, Want: 5},
		{Desc: "two items", Items: []int{5, 7}, Want: 12},
	}) {
		gotest.Equal(it, tc.Want, Total(tc.Items))
	}
}
