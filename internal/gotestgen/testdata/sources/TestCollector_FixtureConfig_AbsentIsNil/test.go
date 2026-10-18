package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PlainFixture struct{}

func (f *PlainFixture) BeforeAll(ctx context.Context) error { return nil }

// UsesTestSuite binds the fixture.
type UsesTestSuite struct {
	Fix *PlainFixture
}

func (s *UsesTestSuite) TestRuns(t *gotest.T) {}
