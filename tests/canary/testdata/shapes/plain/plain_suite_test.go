package plain

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture composed through a named pointer field.
type StoreFixture struct{ Opened bool }

func (f *StoreFixture) BeforeAll(ctx context.Context) error { f.Opened = true; return nil }

// counter is a plain helper type with no name gotest reads.
type counter struct{ n int }

func (c *counter) bump() int { c.n++; return c.n }

// PlainTestSuite binds a fixture through a named pointer field.
type PlainTestSuite struct {
	Store *StoreFixture
}

func (s *PlainTestSuite) TestOpened(t *gotest.T) { gotest.True(t, s.Store.Opened) }

// EmbedsPlainTestSuite embeds a helper type gotest has no interest in.
type EmbedsPlainTestSuite struct {
	counter
}

func (s *EmbedsPlainTestSuite) TestBumps(t *gotest.T) { gotest.Equal(t, 1, s.bump()) }
