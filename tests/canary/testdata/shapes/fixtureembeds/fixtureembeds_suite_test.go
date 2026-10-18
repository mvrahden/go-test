package fixtureembeds

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DBFixture is a package fixture.
type DBFixture struct{}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

// CacheFixture embeds its dependency instead of naming a pointer field.
type CacheFixture struct {
	*DBFixture
}

func (f *CacheFixture) BeforeAll(ctx context.Context) error { return nil }

// BoundTestSuite binds the embedding fixture.
type BoundTestSuite struct {
	Cache *CacheFixture
}

func (s *BoundTestSuite) TestRuns(t *gotest.T) {}
