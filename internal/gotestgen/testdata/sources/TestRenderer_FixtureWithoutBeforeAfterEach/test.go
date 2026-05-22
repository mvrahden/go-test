package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MinimalFixture struct {}

func (f *MinimalFixture) BeforeAll(ctx context.Context) error { return nil }

type MinimalTestSuite struct {
	*MinimalFixture
}

func (s *MinimalTestSuite) TestOne(t *gotest.T) {}
