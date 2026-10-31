package fieldalias

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture.
type StoreFixture struct{ Opened bool }

func (f *StoreFixture) BeforeAll(ctx context.Context) error { f.Opened = true; return nil }

// Store names the fixture struct, not a pointer to it.
type Store = StoreFixture

// AliasTestSuite holds its fixture by value through an alias.
type AliasTestSuite struct {
	Store Store
}

func (s *AliasTestSuite) TestOpened(t *gotest.T) { gotest.True(t, s.Store.Opened) }
