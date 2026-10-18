package grouped

import "github.com/mvrahden/go-test/pkg/gotest"

type (
	// GroupedTestSuite is declared inside a type block.
	GroupedTestSuite struct{}
	other            int
)

func (s *GroupedTestSuite) TestRuns(t *gotest.T) {}

var _ other
