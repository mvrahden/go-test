package fieldaliasptr

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture.
type StoreFixture struct{ Opened bool }

func (f *StoreFixture) BeforeAll(ctx context.Context) error { f.Opened = true; return nil }

// Store names a pointer to the fixture.
type Store = *StoreFixture

// AliasPtrTestSuite binds its fixture through an alias of the pointer.
type AliasPtrTestSuite struct {
	Store Store
}

func (s *AliasPtrTestSuite) TestOpened(t *gotest.T) { gotest.True(t, s.Store.Opened) }
