package gotestgen_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// MethodLoopTestSuite runs generated Test wrappers whose method loop lives in
// gotestruntime.RunMethods, and checks it keeps the per-method lifecycle.
type MethodLoopTestSuite struct{}

func (s *MethodLoopTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Measured on Windows CI: slowest method ~47s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, Timeout: 100 * time.Second}
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
	t.It("still runs the AfterEach of the dependency whose BeforeEach returned", func(it *gotest.T) {
		gotest.Contains(it, run.output, "MARK:fixture before")
		gotest.Contains(it, run.output, "MARK:fixture after")
	})
	t.It("locates the failure at the hook's declaration, not inside gotest", func(it *gotest.T) {
		// test.go:28 declares BrokenHookFixture.BeforeEach in the fixture source.
		gotest.Regexp(it, `(?m)^\s+test\.go:28: BrokenHookFixture\.BeforeEach failed: boom`, run.output)
		gotest.NotContains(it, run.output, "methods.go:", run.output)
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

func (s *MethodLoopTestSuite) TestAsyncFatalEndsTheWait(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^TestFatalAsyncTestSuite$")

	t.It("fails the method the moment its goroutine failed, well before the budget", func(it *gotest.T) {
		gotest.False(it, run.passed, run.output)
		gotest.Contains(it, run.output, "the goroutine that owns done fails")
		// The run's own time, not the build before it: the budget is 20s.
		gotest.Less(it, run.elapsed, 10*time.Second)
	})
}

func (s *MethodLoopTestSuite) TestGoroutineInBeforeAll(t *gotest.T) {
	for _, c := range []struct {
		kind string
		args []string
	}{
		{"test", []string{"-test.run=^TestServerTestSuite$"}},
		{"benchmark", []string{"-test.run=^$", "-test.bench=^BenchmarkServerTestSuite$", "-test.benchtime=1x"}},
	} {
		t.It("waits for a goroutine BeforeAll started only after AfterAll stopped it, in a "+c.kind+" wrapper", func(it *gotest.T) {
			run := runGeneratedSuiteArgs(it, "TestLifecycle_MethodLoop", childTimeout, c.args...)
			assertNoDeadlock(it, run)
			gotest.True(it, run.passed, run.output)
			gotest.Contains(it, run.output, "MARK:server stopped")
		})
	}
}

func (s *MethodLoopTestSuite) TestGoexitInGoroutine(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^TestGoexitTestSuite$")

	t.It("fails the method at the wait, and nothing after the wait runs", func(it *gotest.T) {
		gotest.False(it, run.passed, run.output)
		gotest.Contains(it, run.output, "from the goroutine")
		gotest.NotContains(it, run.output, "MARK:after wait")
	})
}

func (s *MethodLoopTestSuite) TestFuzzHooksMirrorTestHooks(t *gotest.T) {
	for _, c := range []struct{ how, suite, mark string }{
		{"fails", "FuzzBrokenBeforeTestSuite", "MARK:fuzz after broken"},
		{"panics", "FuzzPanicBeforeTestSuite", "MARK:fuzz after panic"},
	} {
		t.It("runs a fuzz execution's AfterEach when its BeforeEach "+c.how, func(it *gotest.T) {
			run := runGeneratedSuiteArgs(it, "TestLifecycle_MethodLoop", childTimeout, "-test.run=^Fuzz"+c.suite+"_FuzzOne$")
			gotest.False(it, run.passed, run.output)
			gotest.Contains(it, run.output, c.mark)
		})
	}
}

func (s *MethodLoopTestSuite) TestPanicAheadOfAFailingAfterEach(t *gotest.T) {
	for _, c := range []struct {
		kind string
		args []string
	}{
		{"test method", []string{"-test.run=^TestPanicAheadTestSuite$"}},
		{"benchmark", []string{"-test.run=^$", "-test.bench=^BenchmarkPanicAheadTestSuite$", "-test.benchtime=1x"}},
		{"fuzz execution", []string{"-test.run=^FuzzPanicAheadTestSuite_FuzzOne$"}},
	} {
		t.It("reports a "+c.kind+"'s panic after an AfterEach that fails fatally", func(it *gotest.T) {
			run := runGeneratedSuiteArgs(it, "TestLifecycle_MethodLoop", childTimeout, c.args...)
			gotest.False(it, run.passed, run.output)
			gotest.Contains(it, run.output, "MARK:after the panic")
			gotest.Contains(it, run.output, "panic: PANIC-AHEAD-OF-AFTEREACH")
		})
	}
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
