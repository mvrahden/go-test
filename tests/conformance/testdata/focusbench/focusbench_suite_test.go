package focusbench_test

import "github.com/mvrahden/go-test/pkg/gotest"

// LoopTestSuite holds the run's only focused method, a benchmark.
type LoopTestSuite struct{}

func (s *LoopTestSuite) TestPlain(t *gotest.T) {}

func (s *LoopTestSuite) F_BenchmarkLoop(b *gotest.B) {
	for b.Loop() {
	}
}
