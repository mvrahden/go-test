// Package stock keeps counts of items on a shelf.
package stock

import "fmt"

// Shelf holds a count per SKU.
type Shelf struct {
	counts map[string]int
}

// NewShelf returns an empty shelf.
func NewShelf() *Shelf { return &Shelf{counts: map[string]int{}} }

// Add puts n items of sku on the shelf.
func (s *Shelf) Add(sku string, n int) { s.counts[sku] += n }

// Take removes n items of sku, or fails when the shelf holds fewer.
func (s *Shelf) Take(sku string, n int) error {
	if s.counts[sku] < n {
		return fmt.Errorf("take %d of %s: only %d on the shelf", n, sku, s.counts[sku])
	}
	s.counts[sku] -= n
	return nil
}

// Count reports how many items of sku are on the shelf.
func (s *Shelf) Count(sku string) int { return s.counts[sku] }
