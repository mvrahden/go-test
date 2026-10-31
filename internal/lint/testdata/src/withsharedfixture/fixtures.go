package withsharedfixture

import "context"

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
