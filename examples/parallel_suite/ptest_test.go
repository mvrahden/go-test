package parallelsuite

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelTestSuite demonstrates the new parallel suite convention using SuiteConfig.
type ParallelTestSuite struct{}

func (s *ParallelTestSuite) BeforeEach(t *gotest.T) {
	Increment()
}

// TestAlpha runs within the suite.
func (s *ParallelTestSuite) TestAlpha(t *gotest.T) {
	Increment()
}

// TestBeta runs within the suite.
func (s *ParallelTestSuite) TestBeta(t *gotest.T) {
	Increment()
}

// TestGamma runs within the suite.
func (s *ParallelTestSuite) TestGamma(t *gotest.T) {
	Increment()
}
