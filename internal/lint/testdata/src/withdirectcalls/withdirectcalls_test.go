package withdirectcalls

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DirectCallTestSuite struct{}

func (s *DirectCallTestSuite) TestParallelDirect(t *gotest.T) {
	t.T().Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}

func (s *DirectCallTestSuite) TestParallelIndirect(t *gotest.T) {
	tt := t.T()
	tt.Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}

func (s *DirectCallTestSuite) TestRunDirect(t *gotest.T) {
	t.T().Run("sub", func(st *testing.T) {}) // want `use It or When instead — T.Run bypasses gotest wrapping`
}

func (s *DirectCallTestSuite) TestRunIndirect(t *gotest.T) {
	tt := t.T()
	tt.Run("sub", func(st *testing.T) {}) // want `use It or When instead — T.Run bypasses gotest wrapping`
}

func (s *DirectCallTestSuite) TestParallelInClosure(t *gotest.T) {
	_ = func() {
		t.T().Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
	}
}

func (s *DirectCallTestSuite) TestRunInClosure(t *gotest.T) {
	_ = func() {
		t.T().Run("sub", func(st *testing.T) {}) // want `use It or When instead — T.Run bypasses gotest wrapping`
	}
}

func (s *DirectCallTestSuite) TestHelperParallel(t *gotest.T) {
	helperParallel(t.T())
}

func (s *DirectCallTestSuite) TestHelperRun(t *gotest.T) {
	helperRun(t.T())
}

func (s *DirectCallTestSuite) TestHelperGotestParallel(t *gotest.T) {
	helperGotestParallel(t)
}

func (s *DirectCallTestSuite) TestHelperGotestRun(t *gotest.T) {
	helperGotestRun(t)
}

func (s *DirectCallTestSuite) TestChainedParallel(t *gotest.T) {
	wrapperParallel(t)
}

func (s *DirectCallTestSuite) TestHelperParallelSecondParam(t *gotest.T) {
	helperParallelSecondParam("ctx", t.T())
}

func (s *DirectCallTestSuite) TestDeepChainParallel(t *gotest.T) {
	deepOuterParallel(t.T())
}

func (s *DirectCallTestSuite) TestMethodHelperParallel(t *gotest.T) {
	var h parallelHelper
	h.doParallel(t.T())
}

func (s *DirectCallTestSuite) TestHelperParallelInClosureScope(t *gotest.T) {
	_ = func() {
		helperParallelClosureOnly(t.T())
	}
}

func (s *DirectCallTestSuite) TestClean(t *gotest.T) {
	gotest.True(t, true)
}

func (s *DirectCallTestSuite) BeforeEach() {}
func (s *DirectCallTestSuite) AfterEach()  {}

func helperParallel(t *testing.T) {
	t.Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}

func helperRun(t *testing.T) {
	t.Run("sub", func(st *testing.T) {}) // want `use It or When instead — T.Run bypasses gotest wrapping`
}

func helperGotestParallel(t *gotest.T) {
	t.T().Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}

func helperGotestRun(t *gotest.T) {
	t.T().Run("sub", func(st *testing.T) {}) // want `use It or When instead — T.Run bypasses gotest wrapping`
}

func wrapperParallel(t *gotest.T) {
	helperParallel(t.T())
}

// Standalone functions must not be flagged for suite-only rules.
func standaloneParallel(t *testing.T) { //nolint:unused
	t.Parallel()
}

func standaloneRun(t *testing.T) { //nolint:unused
	t.Run("sub", func(st *testing.T) {})
}

func helperParallelSecondParam(name string, t *testing.T) {
	t.Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}

func deepLeafParallel(t *testing.T) {
	t.Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}

func deepMiddleParallel(t *testing.T) {
	deepLeafParallel(t)
}

func deepOuterParallel(t *testing.T) {
	deepMiddleParallel(t)
}

type parallelHelper struct{}

func (h *parallelHelper) doParallel(t *testing.T) {
	t.Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}

func helperParallelClosureOnly(t *testing.T) {
	t.Parallel() // want `use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination`
}
