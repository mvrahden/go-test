package parallelsuite

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelTestSuiteParallel is a parallel suite; the generated test function
// calls t.Parallel() so it runs concurrently with other top-level tests.
type ParallelTestSuiteParallel struct{}

func (s *ParallelTestSuiteParallel) BeforeEach(t *gotest.T) {
	Increment()
}

// TestParallelAlpha runs as a parallel subtest within the suite.
func (s *ParallelTestSuiteParallel) TestParallelAlpha(t *gotest.T) {
	Increment()
}

// TestParallelBeta runs as a parallel subtest within the suite.
func (s *ParallelTestSuiteParallel) TestParallelBeta(t *gotest.T) {
	Increment()
}

// TestSequentialGamma runs sequentially within the parallel suite.
func (s *ParallelTestSuiteParallel) TestSequentialGamma(t *gotest.T) {
	Increment()
}
