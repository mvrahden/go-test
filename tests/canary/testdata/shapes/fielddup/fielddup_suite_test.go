package fielddup

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture.
type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

// DupTestSuite names the same fixture twice.
type DupTestSuite struct {
	A, B *StoreFixture
}

func (s *DupTestSuite) TestRuns(t *gotest.T) {}
