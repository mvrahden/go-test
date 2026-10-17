// Package store manages inventory for a small shop daemon.
package store

import (
	"fmt"
	"sync"
	"time"
)

// Inventory tracks stock counts per SKU. Safe for concurrent use.
type Inventory struct {
	mu    sync.Mutex
	stock map[string]int
}

func NewInventory() *Inventory {
	return &Inventory{stock: map[string]int{}}
}

func (i *Inventory) Add(sku string, qty int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.stock[sku] += qty
}

// Reserve removes qty units of sku, or errors if stock is insufficient.
func (i *Inventory) Reserve(sku string, qty int) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.stock[sku] < qty {
		return fmt.Errorf("insufficient stock for %s: have %d, want %d", sku, i.stock[sku], qty)
	}
	i.stock[sku] -= qty
	return nil
}

func (i *Inventory) Stock(sku string) int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.stock[sku]
}

// Catalog is implemented by price sources.
type Catalog interface {
	// Price returns the unit price in cents and whether the SKU is known.
	Price(sku string) (int, bool)
}

// StaticCatalog is an in-memory Catalog.
type StaticCatalog map[string]int

func (c StaticCatalog) Price(sku string) (int, bool) {
	p, ok := c[sku]
	return p, ok
}

// Restocker asynchronously tops up a SKU's stock on an interval.
type Restocker struct {
	inv      *Inventory
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

func NewRestocker(inv *Inventory, interval time.Duration) *Restocker {
	return &Restocker{inv: inv, interval: interval, stop: make(chan struct{}), done: make(chan struct{})}
}

// Start refills sku up to upTo units every interval until Stop is called.
func (r *Restocker) Start(sku string, upTo int) {
	go func() {
		defer close(r.done)
		t := time.NewTicker(r.interval)
		defer t.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-t.C:
				if have := r.inv.Stock(sku); have < upTo {
					r.inv.Add(sku, upTo-have)
				}
			}
		}
	}()
}

func (r *Restocker) Stop() {
	close(r.stop)
	<-r.done
}
