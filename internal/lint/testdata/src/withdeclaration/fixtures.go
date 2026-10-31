package withdeclaration

import "context"

type (
	// CacheSharedFixture is bound from other packages.
	CacheSharedFixture struct{ Addr string } // want `fixture CacheSharedFixture is declared inside a type block; give it its own type declaration`
	other              int
)

func (f *CacheSharedFixture) BeforeAll(context.Context) error { return nil }
