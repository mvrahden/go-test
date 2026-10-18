package lintdeclgrouped

import "github.com/mvrahden/go-test/pkg/gotest"

type (
	counter int

	// GroupedTestSuite is declared inside a type block.
	GroupedTestSuite struct {
		n counter
	}
)

func (s *GroupedTestSuite) TestCounts(t *gotest.T) {
	s.n++
	gotest.Equal(t, counter(1), s.n)
}
