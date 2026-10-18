package withhookcall

import "context"

type CacheSharedFixture struct{ Addr string }

func (f *CacheSharedFixture) BeforeAll(context.Context) error { return nil }
func (f *CacheSharedFixture) Hydrate(context.Context) error   { return nil }
func (f *CacheSharedFixture) Dehydrate(context.Context) error { return nil }
