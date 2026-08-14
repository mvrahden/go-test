package withsharedfixture

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CacheSharedFixture is reachable through QueueSharedFixture's DAG below.
type CacheSharedFixture struct {
	Addr string
}

func (f *CacheSharedFixture) BeforeAll(_ context.Context) error { return nil }
func (f *CacheSharedFixture) AfterAll(_ context.Context) error  { return nil }

// QueueSharedFixture depends on the cache via a fixture-typed field.
type QueueSharedFixture struct {
	Addr  string
	Cache *CacheSharedFixture
}

func (f *QueueSharedFixture) BeforeAll(_ context.Context) error { return nil }
func (f *QueueSharedFixture) AfterAll(_ context.Context) error  { return nil }

func NewCacheSharedFixture() *CacheSharedFixture { return &CacheSharedFixture{} }

var packageCache = &CacheSharedFixture{Addr: "localhost:0"}

var packageQueue = &QueueSharedFixture{}

// DeclaredTestSuite reaches both fixtures through its declared field.
type DeclaredTestSuite struct {
	Queue *QueueSharedFixture
}

func (s *DeclaredTestSuite) TestReachesDeclaredDAG(t *gotest.T) {
	gotest.NotZero(t, s.Queue.Addr)
	gotest.NotZero(t, s.Queue.Cache.Addr)
	cache := s.Queue.Cache
	gotest.NotZero(t, cache.Addr)
}

// UndeclaredTestSuite declares no fixture fields at all.
type UndeclaredTestSuite struct{}

func (s *UndeclaredTestSuite) TestReadsPackageFixture(t *gotest.T) {
	gotest.NotZero(t, packageCache.Addr) // want `suite UndeclaredTestSuite uses \*CacheSharedFixture without declaring it`
}

func (s *UndeclaredTestSuite) TestSuppressed(t *gotest.T) {
	gotest.NotZero(t, packageCache.Addr) //nolint:shared-fixture-undeclared
}

// PartialTestSuite declares the cache but also reads the queue.
type PartialTestSuite struct {
	Cache *CacheSharedFixture
}

func (s *PartialTestSuite) TestReadsUndeclaredQueue(t *gotest.T) {
	gotest.NotZero(t, s.Cache.Addr)
	gotest.NotZero(t, packageQueue.Addr) // want `suite PartialTestSuite uses \*QueueSharedFixture without declaring it`
}

// FixtureSelfTestSuite builds the fixture it tests — locally-constructed
// values never need a window.
type FixtureSelfTestSuite struct{}

func (s *FixtureSelfTestSuite) TestConstructsItsOwn(t *gotest.T) {
	lit := &CacheSharedFixture{Addr: "localhost:1"}
	gotest.NotZero(t, lit.Addr)
	built := NewCacheSharedFixture()
	gotest.NotZero(t, built.Addr)
	alias := lit
	gotest.NotZero(t, alias.Addr)
}
