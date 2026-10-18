package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BaseFixture struct{}

func (f *BaseFixture) BeforeAll(ctx context.Context) error { return nil }

type DBFixture struct {
	BaseFixture *BaseFixture
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

// UsesTestSuite binds the fixture.
type UsesTestSuite struct {
	Fix *DBFixture
}

func (s *UsesTestSuite) TestRuns(t *gotest.T) {}
