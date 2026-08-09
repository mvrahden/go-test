package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type exCtx struct{ val string }

// AllExcludedTestSuite is method-parallel, but every one of its test methods is
// X_-excluded. Such a suite stays in EffectiveTestSuites with an empty TestCases
// slice, so the harness declares no ƒfailed flag and must not import sync/atomic.
type AllExcludedTestSuite struct{}

func (s *AllExcludedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *AllExcludedTestSuite) BeforeEach(t *gotest.T) *exCtx     { return &exCtx{} }
func (s *AllExcludedTestSuite) AfterEach(t *gotest.T, ctx *exCtx) {}
func (s *AllExcludedTestSuite) X_TestOne(t *gotest.T, ctx *exCtx) {}
