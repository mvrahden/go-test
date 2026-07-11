package datarace

import "sync"

type Counter struct {
	n int
}

func (c *Counter) Increment() { c.n++ }
func (c *Counter) Value() int { return c.n }

type SafeCounter struct {
	mu sync.Mutex
	n  int
}

func (c *SafeCounter) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func (c *SafeCounter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func NthElement(s []string, i int) string {
	return s[i]
}

func NilSlice() []string {
	return nil
}
