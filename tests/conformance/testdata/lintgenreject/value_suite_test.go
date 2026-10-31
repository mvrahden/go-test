package lintgenreject

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture.
type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *StoreFixture) AfterAll(ctx context.Context) error { return nil }

// ValueTestSuite holds the fixture by value instead of through a pointer.
type ValueTestSuite struct {
	Store StoreFixture
}

func (s *ValueTestSuite) TestRuns(t *gotest.T) {}
