package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type AppFixture struct {}

func (f *AppFixture) BeforeAll(ctx context.Context) error { return nil }

type BoundTestSuite struct {
	*AppFixture
}

func (s *BoundTestSuite) TestBound(t *gotest.T) {}

type StandaloneTestSuite struct {}

func (s *StandaloneTestSuite) TestFree(t *gotest.T) {}
