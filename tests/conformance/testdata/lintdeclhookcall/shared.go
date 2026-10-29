package lintdeclhookcall

import "context"

// CacheSharedFixture connects in its own setup through Hydrate, which the
// process that owns it never runs.
type CacheSharedFixture struct{ Addr string }

func (f *CacheSharedFixture) BeforeAll(ctx context.Context) error { return f.Hydrate(ctx) }

func (f *CacheSharedFixture) AfterAll(ctx context.Context) error { return f.Dehydrate(ctx) }

func (f *CacheSharedFixture) Hydrate(context.Context) error { return nil }

func (f *CacheSharedFixture) Dehydrate(context.Context) error { return nil }
