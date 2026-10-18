package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *DBFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.DefaultFixtureConfig()
}

// UsesTestSuite binds the fixture.
type UsesTestSuite struct {
	Fix *DBFixture
}

func (s *UsesTestSuite) TestRuns(t *gotest.T) {}
