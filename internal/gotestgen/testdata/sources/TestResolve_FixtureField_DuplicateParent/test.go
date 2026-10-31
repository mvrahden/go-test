package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

type CacheFixture struct {
	A *StoreFixture
	B *StoreFixture
}

func (f *CacheFixture) BeforeAll(ctx context.Context) error { return nil }

type CacheTestSuite struct {
	Cache *CacheFixture
}

func (s *CacheTestSuite) TestRuns(t *gotest.T) {}
