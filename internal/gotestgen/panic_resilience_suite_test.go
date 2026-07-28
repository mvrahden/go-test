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
