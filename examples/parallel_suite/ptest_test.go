package parallelsuite

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelTestSuite demonstrates default suite-level parallelism (always-on).
type ParallelTestSuite struct{}

func (s *ParallelTestSuite) BeforeEach(t *gotest.T) {
	Increment()
}

func (s *ParallelTestSuite) TestAlpha(t *gotest.T) {
	Increment()
}

func (s *ParallelTestSuite) TestBeta(t *gotest.T) {
	Increment()
}

func (s *ParallelTestSuite) TestGamma(t *gotest.T) {
	Increment()
}

// MethodParallelCtx is the per-test context returned by BeforeEach.
type MethodParallelCtx struct {
	Value int64
}

// MethodParallelTestSuite demonstrates method-level parallelism with typed context.
type MethodParallelTestSuite struct{}

func (s *MethodParallelTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *MethodParallelTestSuite) BeforeEach(t *gotest.T) *MethodParallelCtx {
	return &MethodParallelCtx{Value: Increment()}
}

func (s *MethodParallelTestSuite) AfterEach(t *gotest.T, ctx *MethodParallelCtx) {
}

func (s *MethodParallelTestSuite) TestOne(t *gotest.T, ctx *MethodParallelCtx) {
	Increment()
}

func (s *MethodParallelTestSuite) TestTwo(t *gotest.T, ctx *MethodParallelCtx) {
	Increment()
}
