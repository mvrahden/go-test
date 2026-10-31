package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

type DupTestSuite struct {
	A, B *StoreFixture
}

func (s *DupTestSuite) TestRuns(t *gotest.T) {}
