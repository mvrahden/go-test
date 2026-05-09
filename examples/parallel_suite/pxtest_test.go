package parallelsuite_test

import (
	parallelsuite "github.com/mvrahden/go-test/examples/parallel_suite"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelExtTestSuite demonstrates the new parallel suite convention using SuiteConfig.
type ParallelExtTestSuite struct{}

func (s *ParallelExtTestSuite) BeforeEach(t *gotest.T) {
	parallelsuite.Increment()
}

// TestDelta runs within the suite.
func (s *ParallelExtTestSuite) TestDelta(t *gotest.T) {
	parallelsuite.Increment()
}

// TestEpsilon runs within the suite.
func (s *ParallelExtTestSuite) TestEpsilon(t *gotest.T) {
	parallelsuite.Increment()
}

// TestZeta runs within the suite.
func (s *ParallelExtTestSuite) TestZeta(t *gotest.T) {
	parallelsuite.Increment()
}
