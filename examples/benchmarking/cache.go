// Package benchmarking is a fixed-capacity LRU cache sitting in front of a
// slow store — the kind of thing almost every service ends up writing, and
// the one place "is this allocation-free?" is a question people actually
// ask about their hot path.
package benchmarking

// entry is a node in an intrusive doubly linked list that tracks recency.
// Hand-rolling this (rather than reaching for container/list, whose Element
// stores its payload as `any` and boxes it on every insertion) is what keeps
// Get's hot path allocation-free: promoting an entry to the front is just a
// handful of pointer writes, never a heap allocation.
type entry struct {
	key, value string
	prev, next *entry
}

// Cache is a fixed-capacity, in-process LRU cache. It is not safe for
// concurrent use.
type Cache struct {
	capacity int
	items    map[string]*entry

	// head is the most recently used entry, tail the least recently used.
	head, tail *entry
}

// New creates a Cache holding at most capacity entries. A non-positive
// capacity is treated as 1.
func New(capacity int) *Cache {
	if capacity < 1 {
		capacity = 1
	}
	return &Cache{
		capacity: capacity,
		items:    make(map[string]*entry, capacity),
	}
}

// Get looks up key and, on a hit, promotes it to most-recently-used.
func (c *Cache) Get(key string) (string, bool) {
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.moveToFront(e)
	return e.value, true
}

// Put inserts or updates key. Updating an existing key also promotes it to
// most-recently-used. Inserting past capacity evicts the least recently
// used entry first.
func (c *Cache) Put(key, value string) {
	if e, ok := c.items[key]; ok {
		e.value = value
		c.moveToFront(e)
		return
	}
	if len(c.items) >= c.capacity {
		c.evictTail()
	}
	e := &entry{key: key, value: value}
	c.items[key] = e
	c.pushFront(e)
}

// Len reports the number of entries currently cached.
func (c *Cache) Len() int {
	return len(c.items)
}

func (c *Cache) pushFront(e *entry) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *Cache) unlink(e *entry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

func (c *Cache) moveToFront(e *entry) {
	if c.head == e {
		return
	}
	c.unlink(e)
	c.pushFront(e)
}

func (c *Cache) evictTail() {
	e := c.tail
	if e == nil {
		return
	}
	c.unlink(e)
	delete(c.items, e.key)
}
