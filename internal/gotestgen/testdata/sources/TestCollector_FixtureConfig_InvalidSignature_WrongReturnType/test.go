package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BadFixture struct{}

func (f *BadFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BadFixture) FixtureConfig() string               { return "" }

// UsesTestSuite binds the fixture.
type UsesTestSuite struct {
	Fix *BadFixture
}

func (s *UsesTestSuite) TestRuns(t *gotest.T) {}
