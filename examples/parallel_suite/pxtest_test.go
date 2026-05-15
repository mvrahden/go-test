package parallelsuite_test

import "github.com/mvrahden/go-test/pkg/gotest"

// ExtParallelCtx holds per-test state for the external parallel suite.
type ExtParallelCtx struct {
	Value int
}

// ParallelExtTestSuite demonstrates method-level parallelism in an external
// test package.
type ParallelExtTestSuite struct{}

func (s *ParallelExtTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ParallelExtTestSuite) BeforeEach(t *gotest.T) *ExtParallelCtx {
	return &ExtParallelCtx{Value: 42}
}

func (s *ParallelExtTestSuite) AfterEach(t *gotest.T, ctx *ExtParallelCtx) {}

func (s *ParallelExtTestSuite) TestDelta(t *gotest.T, ctx *ExtParallelCtx) {
	gotest.Equal(t, 42, ctx.Value)
}

func (s *ParallelExtTestSuite) TestEpsilon(t *gotest.T, ctx *ExtParallelCtx) {
	gotest.NotZero(t, ctx.Value)
}
