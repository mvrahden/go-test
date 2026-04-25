package parallelsuite_test

import (
	parallelsuite "github.com/mvrahden/go-test/examples/parallel_suite"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelExtTestSuiteParallel is a parallel external-package suite; the
// generated test function calls t.Parallel() so it runs concurrently with
// other top-level tests.
type ParallelExtTestSuiteParallel struct{}

func (s *ParallelExtTestSuiteParallel) BeforeEach(t *gotest.T) {
	parallelsuite.Increment()
}

// TestParallelDelta runs as a parallel subtest within the suite.
func (s *ParallelExtTestSuiteParallel) TestParallelDelta(t *gotest.T) {
	parallelsuite.Increment()
}

// TestParallelEpsilon runs as a parallel subtest within the suite.
func (s *ParallelExtTestSuiteParallel) TestParallelEpsilon(t *gotest.T) {
	parallelsuite.Increment()
}

// TestSequentialZeta runs sequentially within the parallel suite.
func (s *ParallelExtTestSuiteParallel) TestSequentialZeta(t *gotest.T) {
	parallelsuite.Increment()
}
