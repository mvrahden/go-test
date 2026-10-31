package fieldvalue

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture.
type StoreFixture struct{ Opened bool }

func (f *StoreFixture) BeforeAll(ctx context.Context) error { f.Opened = true; return nil }

// ValueTestSuite holds its fixture by value.
type ValueTestSuite struct {
	Store StoreFixture
}

func (s *ValueTestSuite) TestOpened(t *gotest.T) { gotest.True(t, s.Store.Opened) }
