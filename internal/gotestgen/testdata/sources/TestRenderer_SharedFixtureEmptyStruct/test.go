package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SetupSharedFixture struct{}

func (f *SetupSharedFixture) BeforeAll(ctx context.Context) error { return nil }

type AppFixture struct {
	*SetupSharedFixture
}

func (f *AppFixture) BeforeAll(ctx context.Context) error { return nil }

type AppTestSuite struct {
	*AppFixture
}

func (s *AppTestSuite) TestRun(t *gotest.T) {}
