package gotestgen_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// MethodLoopTestSuite runs generated Test wrappers whose method loop lives in
// gotestruntime.RunMethods, and checks it keeps the per-method lifecycle.
type MethodLoopTestSuite struct{}

func (s *MethodLoopTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	cfg.Timeout = 3 * time.Minute
	return cfg
}

func (s *MethodLoopTestSuite) TestHookOrder(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^TestOrderTestSuite$")

	t.It("wraps the suite's hooks in the fixture's around the method", func(it *gotest.T) {
		gotest.True(it, run.passed, run.output)
		gotest.Regexp(it, `(?s)MARK:fixture before.*MARK:suite before.*MARK:method.*MARK:suite after.*MARK:fixture after`, run.output)
	})
}

func (s *MethodLoopTestSuite) TestFixtureBeforeEachFails(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^TestBrokenHookTestSuite$")

	t.It("fails the method without running it, and the fixture's AfterEach, paired with nothing, does not run", func(it *gotest.T) {
		gotest.False(it, run.passed, run.output)
		gotest.Contains(it, run.output, "BrokenHookFixture.BeforeEach failed: boom")
		gotest.NotContains(it, run.output, "MARK:never")
		gotest.NotContains(it, run.output, "MARK:broken after")
	})
}

func (s *MethodLoopTestSuite) TestFailFast(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^TestFailFastTestSuite$")

	t.It("starts no method after the first failure", func(it *gotest.T) {
		gotest.False(it, run.passed, run.output)
		gotest.Contains(it, run.output, "--- FAIL: TestFailFastTestSuite/TestA")
		gotest.NotContains(it, run.output, "MARK:after failfast")
	})
}

func (s *MethodLoopTestSuite) TestAsyncDeadline(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^TestAsyncTestSuite$")

	t.It("fails an async method that never calls done", func(it *gotest.T) {
		assertNoDeadlock(it, run)
		gotest.False(it, run.passed, run.output)
		gotest.Contains(it, run.output, "TestNeverDoneAsync: done() was not called before the test deadline")
	})
	t.It("locates the verdict at the method's declaration, not inside gotest", func(it *gotest.T) {
		// test.go:56 declares TestNeverDoneAsync in the fixture source.
		gotest.Regexp(it, `(?m)^\s+test\.go:56: TestNeverDoneAsync: done\(\) was not called`, run.output)
		gotest.NotContains(it, run.output, "methods.go:", run.output)
	})
}

func (s *MethodLoopTestSuite) TestCallSites(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^TestSiteTestSuite$")

	// The second location is the outermost user frame, which runtime frames
	// between t.Run and the method must not displace.
	t.It("reports the method's line behind a helper's for a returning BeforeEach and an async method", func(it *gotest.T) {
		gotest.False(it, run.passed, run.output)
		gotest.Regexp(it, `TestReturning\n\s+test\.go:75: test\.go:67: Equal failed`, run.output)
		gotest.Regexp(it, `TestWaitsAsync\n\s+test\.go:75: test\.go:72: Equal failed`, run.output)
	})
}
