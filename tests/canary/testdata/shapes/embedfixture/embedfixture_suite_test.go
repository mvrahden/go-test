package embedfixture

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StoreFixture is a package fixture.
type StoreFixture struct{}

func (f *StoreFixture) BeforeAll(ctx context.Context) error { return nil }

// ChildTestSuite embeds the fixture instead of naming a field.
type ChildTestSuite struct {
	*StoreFixture
}

func (s *ChildTestSuite) TestRuns(t *gotest.T) {}
