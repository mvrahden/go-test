package parallel

import "github.com/mvrahden/go-test/pkg/gotest"

// A parallel suite interleaves its events with every other package's in the
// stream. The spec tree has to reassemble them by test path rather than by
// arrival order.
type ParallelTestSuite struct{}

func (s *ParallelTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ParallelTestSuite) TestConcurrentWork(t *gotest.T) {
	t.When("work runs in parallel", func(t *gotest.T) {
		t.It("completes the first unit", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
		t.It("completes the second unit", func(t *gotest.T) {
			gotest.Equal(t, 2, 2)
		})
		t.It("completes the third unit", func(t *gotest.T) {
			gotest.Equal(t, 3, 3)
		})
	})
}
