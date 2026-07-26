package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type AlphaSharedFixture struct {
	Beta *BetaSharedFixture
}

func (f *AlphaSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *AlphaSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type BetaSharedFixture struct {
	Alpha *AlphaSharedFixture
}

func (f *BetaSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BetaSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type CycleTestSuite struct {
	Alpha *AlphaSharedFixture
}

func (s *CycleTestSuite) TestOne(t *gotest.T) {}
