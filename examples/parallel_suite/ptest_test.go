package parallelsuite

import (
	"sync/atomic"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var idSeq atomic.Int64

// MethodParallelCtx holds per-test state for parallel execution.
type MethodParallelCtx struct {
	Value int64
}

// MethodParallelTestSuite demonstrates method-level parallelism.
// SuiteConfig{Parallel: true} makes each test method run concurrently.
// A returning BeforeEach provides per-test state isolation.
type MethodParallelTestSuite struct{}

func (s *MethodParallelTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *MethodParallelTestSuite) BeforeEach(t *gotest.T) *MethodParallelCtx {
	return &MethodParallelCtx{Value: idSeq.Add(1)}
}

func (s *MethodParallelTestSuite) AfterEach(t *gotest.T, ctx *MethodParallelCtx) {}

func (s *MethodParallelTestSuite) TestOne(t *gotest.T, ctx *MethodParallelCtx) {
	gotest.NotZero(t, ctx.Value)
}

func (s *MethodParallelTestSuite) TestTwo(t *gotest.T, ctx *MethodParallelCtx) {
	gotest.NotZero(t, ctx.Value)
}
