package gotestruntime

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// This file holds the wiring that `gotest generate` emits into a suite harness.
// It lives here rather than in pkg/gotest so that suite authors never meet it:
// everything exported from pkg/gotest is something you would deliberately call
// from a test.

// TestCase is the shape of a generated test-method reference.
type TestCase func(*gotest.T)

// SetupT builds the *gotest.T handed to a suite's BeforeAll.
func SetupT(t *testing.T, timeout time.Duration) *gotest.T {
	return testScopedT(t, timeout)
}

// TestT builds the *gotest.T handed to a single test method. The timeout only
// bounds the context here; holding the method to it is [RunTest]'s job, because
// code that ignores its context runs to completion either way.
func TestT(t *testing.T, timeout time.Duration) *gotest.T {
	return testScopedT(t, timeout)
}

// RunTest runs one test method, held to the suite's Timeout if it declared one.
func RunTest(t *gotest.T, cfg SuiteConfig, run func()) {
	watchWhile(t, cfg.TestBudget(), "", "Timeout", run)
}

// RunSetup runs a suite's BeforeAll, held to its SetupTimeout if it declared one.
func RunSetup(t *testing.T, cfg SuiteConfig, beforeAll func(*gotest.T)) {
	tt := SetupT(t, cfg.SetupTimeout)
	watchWhile(tt, cfg.SetupBudget(), "BeforeAll ", "SetupTimeout", func() { beforeAll(tt) })
}

// RunTeardown runs a suite's AfterAll from inside t.Cleanup, on the same terms
// as [RunSetup].
func RunTeardown(t *testing.T, cfg SuiteConfig, afterAll func(*gotest.T)) {
	tt := TeardownT(t, cfg.SetupTimeout)
	watchWhile(tt, cfg.SetupBudget(), "AfterAll ", "SetupTimeout", func() { afterAll(tt) })
}

// watchWhile runs a lifecycle phase and fails t the moment timeout passes with
// that phase still running. what names the phase for the message ("BeforeAll ",
// "AfterAll ", or empty for a test method) and budget names the config field it
// came from.
//
// The alternative — checking the deadline once the phase returns — cannot judge
// a phase that never returns, which is the case that matters most. A wedged
// BeforeAll would leave nothing behind but the -timeout dump, and that names no
// budget at all.
//
// Nothing here interrupts the phase; it cannot. Go has no way to stop another
// goroutine, and running the phase on a goroutine we could abandon is worse than
// it looks: the testing package waits on subtests registered by t.Run, so a hang
// inside a nested When or It would not be bounded anyway, and the orphaned
// subtest would later signal a parent that had already finished. Killing the
// process is the only true interruption, and that is what go test -timeout is.
// What this adds is the verdict.
func watchWhile(t *gotest.T, timeout time.Duration, what, budget string, run func()) {
	if timeout <= 0 {
		run()
		return
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go watchDeadline(t, timeout, what, budget, done, stopped)

	// close(done) also runs when the phase exits via Goexit. Waiting for the
	// watchdog to stop is what makes its Errorf legal: the testing package
	// panics on a log from a test that has already completed, and the test
	// cannot complete while this frame is still on the stack.
	defer func() {
		close(done)
		<-stopped
	}()
	run()
}

// watchDeadline fails t if the phase has not finished within timeout. Errorf is
// safe from another goroutine while the test is still running, which the
// handshake in watchWhile guarantees it is.
func watchDeadline(t *gotest.T, timeout time.Duration, what, budget string, done <-chan struct{}, stopped chan<- struct{}) {
	defer close(stopped)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
	}

	// A select picks at random among the cases that are ready, so arriving here
	// does not prove the phase is still running — it may have finished in the
	// same instant the timer fired. Re-check without blocking: finishing on the
	// deadline is not an overrun, and reporting one would be a coin flip.
	select {
	case <-done:
		return
	default:
	}

	// Written unbuffered as well as recorded on the test. A phase that never
	// returns never completes, and the testing package only flushes a test's
	// output when it does — so if the process is later killed by go test
	// -timeout, this line is the only trace that a budget was blown, and which
	// one.
	fmt.Fprintf(os.Stderr, "gotest: %s %sexceeded its configured %s of %s and is still running\n",
		t.T().Name(), what, budget, timeout)
	t.Errorf("%sexceeded its configured %s of %s and is still running", what, budget, timeout)
}

// testScopedT applies the timeout convention shared by the phases that run while
// the test is still live: a positive timeout becomes a deadline, anything else
// (including the documented -1 "disabled") leaves the context unbounded.
func testScopedT(t *testing.T, timeout time.Duration) *gotest.T {
	if timeout <= 0 {
		return gotest.NewT(t)
	}
	return gotest.NewTWithDeadline(t, timeout)
}

// TeardownT builds the *gotest.T handed to a suite's AfterAll, and must be
// called from inside t.Cleanup.
//
// It deliberately does not inherit the test's cancellation. The testing package
// cancels t.Context() immediately before it runs the cleanup functions, so a
// context derived from it would reach AfterAll already canceled and every
// context-aware teardown would fail instantly. Values carry over; cancellation
// does not; the configured setup/teardown timeout is applied on top.
func TeardownT(t *testing.T, timeout time.Duration) *gotest.T {
	ctx := context.WithoutCancel(t.Context())
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		t.Cleanup(cancel)
	}
	return gotest.NewTWithContext(t, ctx)
}
