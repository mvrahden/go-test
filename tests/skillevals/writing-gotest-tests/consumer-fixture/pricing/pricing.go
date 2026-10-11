// Package pricing computes order totals against a store.Catalog.
package pricing

import (
	"fmt"

	"example.com/shopd/store"
)

// Total sums the unit prices of skus. Unknown SKUs error.
func Total(c store.Catalog, skus []string) (int, error) {
	sum := 0
	for _, sku := range skus {
		p, ok := c.Price(sku)
		if !ok {
			return 0, fmt.Errorf("unknown sku %q", sku)
		}
		sum += p
	}
	return sum, nil
}
