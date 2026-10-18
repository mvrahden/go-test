package fieldunreferenced

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is the fixture the suite binds.
type StoreFixture struct{ Opened bool }

func (f *StoreFixture) BeforeAll(ctx context.Context) error { f.Opened = true; return nil }

// BrokenFixture is no fixture gotest could run, and no suite binds it.
type BrokenFixture struct{}

func (f *BrokenFixture) BeforeAll(ctx context.Context) {}

// LabelFixture is no struct, and no suite binds it.
type LabelFixture string

// BoundTestSuite binds StoreFixture only.
type BoundTestSuite struct {
	Store *StoreFixture
}

func (s *BoundTestSuite) TestOpened(t *gotest.T) { gotest.True(t, s.Store.Opened) }
