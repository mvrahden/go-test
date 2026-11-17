package parallelsetenv

import "github.com/mvrahden/go-test/pkg/gotest"

// MethodEnvTestSuite sets the environment in a method.
type MethodEnvTestSuite struct{}

func (s *MethodEnvTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *MethodEnvTestSuite) TestSets(t *gotest.T) {
	t.Setenv("PARALLELSETENV", "method")
}

// HookEnvTestSuite sets it in its returning BeforeEach.
type HookEnvTestSuite struct{}

type hookCtx struct{}

func (s *HookEnvTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *HookEnvTestSuite) BeforeEach(t *gotest.T) *hookCtx {
	t.Setenv("PARALLELSETENV", "hook")
	return &hookCtx{}
}

func (s *HookEnvTestSuite) TestReads(t *gotest.T, c *hookCtx) {}

// NestedEnvTestSuite sets it on the T of a nested It.
type NestedEnvTestSuite struct{}

func (s *NestedEnvTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *NestedEnvTestSuite) TestSets(t *gotest.T) {
	t.It("sets it", func(it *gotest.T) {
		it.Setenv("PARALLELSETENV", "nested")
	})
}

// StdlibEnvTestSuite sets it through t.T().
type StdlibEnvTestSuite struct{}

func (s *StdlibEnvTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *StdlibEnvTestSuite) TestSets(t *gotest.T) {
	t.T().Setenv("PARALLELSETENV", "stdlib")
}
