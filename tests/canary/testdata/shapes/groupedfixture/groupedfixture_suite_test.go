package groupedfixture

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type (
	// GroupedFixture is declared inside a type block.
	GroupedFixture struct{}
	other          int
)

func (f *GroupedFixture) BeforeAll(ctx context.Context) error { return nil }

// BoundTestSuite binds the grouped fixture.
type BoundTestSuite struct {
	Fix *GroupedFixture
}

func (s *BoundTestSuite) TestRuns(t *gotest.T) {}

var _ other
