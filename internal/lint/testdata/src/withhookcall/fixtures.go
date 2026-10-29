package withhookcall

import "context"

type CacheSharedFixture struct{ Addr string }

// The owning process runs BeforeAll and AfterAll only, so connecting
// through its own Hydrate runs nothing twice.
func (f *CacheSharedFixture) BeforeAll(ctx context.Context) error { return f.Hydrate(ctx) }
func (f *CacheSharedFixture) AfterAll(ctx context.Context) error  { return f.Dehydrate(ctx) }
func (f *CacheSharedFixture) Hydrate(ctx context.Context) error {
	return f.BeforeAll(ctx) // want `BeforeAll is called by the harness; calling it here runs it twice`
}
func (f *CacheSharedFixture) Dehydrate(context.Context) error { return nil }
