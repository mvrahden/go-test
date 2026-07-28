package benchmarking

import (
	"strconv"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CacheTestSuite exercises Cache both for correctness and for performance.
// Corpus is a named field bound to KeyCorpusFixture (see fixtures_test.go),
// built once for the whole suite; cache is rebuilt by BeforeEach below.
type CacheTestSuite struct {
	Corpus *KeyCorpusFixture
	cache  *Cache
}

// BeforeEach rebuilds a cache warmed from the corpus before every test and
// every BenchmarkX method. This is the whole point of the example: gotest's
// generated wrapper fences the timer around a benchmark method's BeforeEach
// call, so this rebuild is never part of the measurement. A hand-written Go
// benchmark reusing this same warm-cache-per-iteration setup would need an
// explicit b.StopTimer() before the rebuild and b.StartTimer() after it —
// easy to forget, and silent when you do. Here it's structural: there is no
// timer to remember to stop.
func (s *CacheTestSuite) BeforeEach(t *gotest.T) {
	s.cache = New(len(s.Corpus.Keys))
	for i, key := range s.Corpus.Keys {
		s.cache.Put(key, s.Corpus.Values[i])
	}
}

func (s *CacheTestSuite) TestEvictsLeastRecentlyUsed(t *gotest.T) {
	t.When("a cache at capacity receives a new key", func(t *gotest.T) {
		cache := New(2)
		cache.Put("a", "1")
		cache.Put("b", "2")
		cache.Get("a") // touch "a" so "b" becomes the least recently used

		cache.Put("c", "3")

		t.It("evicts the least recently used entry", func(t *gotest.T) {
			_, ok := cache.Get("b")
			gotest.False(t, ok)
		})

		t.It("keeps the entries that were used", func(t *gotest.T) {
			v, ok := cache.Get("a")
			gotest.True(t, ok)
			gotest.Equal(t, "1", v)

			v, ok = cache.Get("c")
			gotest.True(t, ok)
			gotest.Equal(t, "3", v)
		})

		t.It("stays within capacity", func(t *gotest.T) {
			gotest.Equal(t, 2, cache.Len())
		})
	})

	t.When("an existing key is put again", func(t *gotest.T) {
		cache := New(2)
		cache.Put("a", "1")
		cache.Put("b", "2")

		cache.Put("a", "1-updated") // update should also promote "a"
		cache.Put("c", "3")         // cache is full again; "b" is now least recently used

		t.It("updates the value", func(t *gotest.T) {
			v, ok := cache.Get("a")
			gotest.True(t, ok)
			gotest.Equal(t, "1-updated", v)
		})

		t.It("promotes the updated key instead of evicting it", func(t *gotest.T) {
			_, ok := cache.Get("b")
			gotest.False(t, ok)

			v, ok := cache.Get("c")
			gotest.True(t, ok)
			gotest.Equal(t, "3", v)
		})
	})
}

// BenchmarkGetHit is the hot path: fetch a key already in the cache. Nothing
// above "for b.Loop()" — including BeforeEach's rebuild of s.cache — is part
// of the measurement; see the comment on BeforeEach for why that matters.
// This is the benchmark to watch for 0 allocs/op.
func (s *CacheTestSuite) BenchmarkGetHit(b *gotest.B) {
	key := s.Corpus.Keys[0]
	for b.Loop() {
		s.cache.Get(key)
	}
}

// BenchmarkGetMiss looks up a key that was never in the cache.
func (s *CacheTestSuite) BenchmarkGetMiss(b *gotest.B) {
	for b.Loop() {
		s.cache.Get("not-a-key")
	}
}

// BenchmarkPutEviction measures steady-state inserts into a cache that is
// already at capacity: BeforeEach warms s.cache from the full corpus, so
// every Put below evicts the current tail before inserting a fresh key.
//
// Its reported allocs/op flips between 1 and 2 run to run at a constant 55
// B/op. That's the metric, not the code: AllocsPerOp() is an integer
// division (total allocs / N), so a true average sitting close to 2 (an
// *entry plus a key string on nearly every call) gets truncated to whichever
// whole number it happens to land nearest to that run — never a fraction.
func (s *CacheTestSuite) BenchmarkPutEviction(b *gotest.B) {
	i := 0
	for b.Loop() {
		s.cache.Put(strconv.Itoa(i), "v")
		i++
	}
}

// fillSize is the number of entries BenchmarkFillFromEmpty inserts per op —
// the fixture's entire corpus, so a full fill uses every generated key once.
const fillSize = corpusSize

// BenchmarkFillFromEmpty measures building a cache from scratch. One op is a
// full fill of fillSize entries into a fresh cache whose capacity is exactly
// fillSize, so no entry can ever be evicted — the contrast against
// BenchmarkPutEviction is structural (a cache that can never be full enough
// to evict), not a matter of how many iterations b.Loop() happens to run.
// A capacity-headroom version of this benchmark was tried first and
// rejected: at default -benchtime it ran millions of iterations against a
// cache with "only" 1<<20 headroom, filled that headroom about a fifth of
// the way through the run, and then evicted for the remaining 77% of
// iterations — silently measuring BenchmarkPutEviction's path instead of
// its own. This shape makes that impossible regardless of iteration count.
//
// Every Put here allocates a new *entry, and one op also allocates the
// destination cache's backing map — so its allocation profile isn't "N
// individual Puts" so much as "one cache's worth of Puts plus one map
// alloc," and its ns/op is the cost of fillSize inserts, not one insert:
// don't compare it directly to BenchmarkPutEviction's or BenchmarkGetHit's
// per-op numbers.
func (s *CacheTestSuite) BenchmarkFillFromEmpty(b *gotest.B) {
	keys := s.Corpus.Keys[:fillSize]
	vals := s.Corpus.Values[:fillSize]
	for b.Loop() {
		c := New(fillSize)
		for i := range keys {
			c.Put(keys[i], vals[i])
		}
	}
}
