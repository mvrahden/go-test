package gotestgen_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PanicResilienceTestSuite guards the lifecycle stages where a panic used to
// escape the testing package entirely — aborting the process from a goroutine
// testing knows nothing about, so no cleanup of any kind ran.
//
// Each case compiles a real generated harness and runs it as a subprocess, so
// what is asserted is the observable behaviour of a whole test binary: what got
// released, what got reported, and that it happened promptly.
type PanicResilienceTestSuite struct{}

func (s *PanicResilienceTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute}
}

func (s *PanicResilienceTestSuite) TestPanicInPollFunction(t *gotest.T) {
	t.When("an Eventually poll function panics while a fixture is held", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_PollPanic")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "a panicking suite must fail:\n%s", run.output)
		})

		w.It("reports the original panic", func(it *gotest.T) {
			gotest.Contains(it, run.output, "boom inside the poll function",
				"the poll function's panic should reach the output:\n%s", run.output)
		})

		w.It("still runs suite and fixture teardown", func(it *gotest.T) {
			// Record runs the poll function on its own goroutine. Left
			// unrecovered there, the panic kills the process before testing can
			// run a single cleanup — the fixture is acquired and never released.
			gotest.Contains(it, run.output, "MARK:suite afterall",
				"AfterAll must still run:\n%s", run.output)
			gotest.Contains(it, run.output, "MARK:fixture released",
				"the fixture must still be released:\n%s", run.output)
		})
	})
}

func (s *PanicResilienceTestSuite) TestPanicInEachAfterRecordedFailure(t *gotest.T) {
	t.When("an Each entry records a failure and then panics", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_EachPanicAfterFailure")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "a panicking suite must fail:\n%s", run.output)
		})

		w.It("does not swallow the panic", func(it *gotest.T) {
			// The deferred FailNow used to run runtime.Goexit while the panic
			// was unwinding, which abandons it: the run failed on the recorded
			// error alone and the panic never appeared anywhere.
			gotest.Contains(it, run.output, "boom after a recorded failure",
				"the panic must not be discarded by the Goexit in eachRun:\n%s", run.output)
		})

		w.It("still reports the recorded failure and runs AfterAll", func(it *gotest.T) {
			gotest.Contains(it, run.output, "recorded a non-fatal failure", run.output)
			gotest.Contains(it, run.output, "MARK:suite afterall", run.output)
		})
	})
}

func (s *PanicResilienceTestSuite) TestPanicInFixtureTeardown(t *gotest.T) {
	t.When("one fixture's AfterAll panics while a sibling is still releasing", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_FixtureTeardownPanic")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
		})

		w.It("lets the sibling finish releasing", func(it *gotest.T) {
			// Teardown runs concurrently across the fixture graph, so an
			// unrecovered panic took every sibling down with it mid-release.
			gotest.Contains(it, run.output, "MARK:slow fixture released",
				"a sibling fixture must still complete its teardown:\n%s", run.output)
		})

		w.It("reports the panic as a teardown failure", func(it *gotest.T) {
			gotest.Contains(it, run.output, "boom in fixture AfterAll", run.output)
			gotest.Contains(it, run.output, "panicked", run.output)
			gotest.Equal(it, 1, strings.Count(run.output, "MARK:test ran"),
				"the test itself should still have run exactly once:\n%s", run.output)
		})
	})
}

func (s *PanicResilienceTestSuite) TestConfiguredTimeoutOverrun(t *gotest.T) {
	t.When("a test blows its configured Timeout while ignoring the context", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_TimeoutOverrun")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
		})

		w.It("fails the run rather than passing silently", func(it *gotest.T) {
			// Go cannot preempt the test, so it runs to completion either way.
			// The budget only means something if the overrun is reported.
			gotest.False(it, run.passed, "an overrun must fail the run:\n%s", run.output)
			gotest.Contains(it, run.output, "exceeded its configured Timeout",
				"the overrun should be reported:\n%s", run.output)
			gotest.Contains(it, run.output, "MARK:overrunning test returned", run.output)
		})

		w.It("names the overrunning test and leaves the others alone", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:fast test returned", run.output)
			gotest.Contains(it, run.output, "TestOverrunsItsBudget exceeded its configured Timeout",
				"the overrun should name the test that blew its budget:\n%s", run.output)
			gotest.NotContains(it, run.output, "TestStaysWithinBudget exceeded",
				"a test within budget must not be reported:\n%s", run.output)
			gotest.Contains(it, run.output, "MARK:suite afterall", run.output)
		})
	})
}

func (s *PanicResilienceTestSuite) TestPanicOnSpawnedGoroutine(t *gotest.T) {
	t.When("a goroutine started with gotest.Go panics", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_GoroutinePanic")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "a panicking suite must fail:\n%s", run.output)
		})

		w.It("reports the panic and still runs every cleanup", func(it *gotest.T) {
			// A bare `go func(){ panic(...) }()` aborts the process with no
			// cleanup whatsoever; the point of gotest.Go is that this does not.
			gotest.Contains(it, run.output, "boom on a spawned goroutine", run.output)
			gotest.Contains(it, run.output, "MARK:suite afterall", run.output)
			gotest.Contains(it, run.output, "MARK:fixture released", run.output)
		})
	})
}

func (s *PanicResilienceTestSuite) TestSetupTimeoutOverrun(t *gotest.T) {
	t.When("BeforeAll blows its configured SetupTimeout", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_SetupOverrun")

		w.It("fails the run rather than passing silently", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "an overrun must fail the run:\n%s", run.output)
			gotest.Contains(it, run.output, "BeforeAll exceeded its configured SetupTimeout", run.output)
		})

		w.It("still runs the tests and AfterAll", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:beforeall returned", run.output)
			gotest.Contains(it, run.output, "MARK:test ran", run.output)
			gotest.Contains(it, run.output, "MARK:afterall ran", run.output)
		})
	})
}
