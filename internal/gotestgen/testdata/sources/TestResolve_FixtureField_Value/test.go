package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

type ValueTestSuite struct {
	Store StoreFixture
}

func (s *ValueTestSuite) TestRuns(t *gotest.T) {}
