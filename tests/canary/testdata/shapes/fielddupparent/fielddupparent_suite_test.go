package fielddupparent

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture.
type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

// CacheFixture names the same parent fixture twice.
type CacheFixture struct {
	A *StoreFixture
	B *StoreFixture
}

func (f *CacheFixture) BeforeAll(ctx context.Context) error { return nil }

// CacheTestSuite binds the fixture with the doubled parent.
type CacheTestSuite struct {
	Cache *CacheFixture
}

func (s *CacheTestSuite) TestRuns(t *gotest.T) {}
