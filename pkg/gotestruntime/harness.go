package gotestruntime

import (
	"context"
	"errors"
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

// TestT builds the *gotest.T handed to a single test method.
//
// Go cannot preempt a running test, so a configured Timeout can only bound the
// context handed to it — code that ignores that context runs to completion. It
// would then pass, silently, having blown the budget the suite asked for. The
// deadline is therefore checked once the method is done and reported as a
// failure, which is the most a deadline can mean here.
func TestT(t *testing.T, timeout time.Duration) *gotest.T {
	return testScopedT(t, timeout)
}

// RunTest runs one test method under its configured deadline.
//
// Nothing can interrupt it. Go has no way to stop another goroutine — no kill,
// no remote panic, no remote Goexit — and running the method on a goroutine we
// could abandon is worse than it looks: the testing package waits on subtests
// registered by t.Run, so a hang inside a nested When or It would not be bounded
// anyway, and the orphaned subtest would later signal a parent that had already
// finished. Killing the process is the only true interruption, and that is what
// go test -timeout already is.
//
// What is missing without this is a verdict. A hung test produces no failure at
// all — just a timeout dump naming no budget — so a watchdog reports the overrun
// while the method is still running. The test is marked failed the moment its
// deadline passes, whatever happens to the process afterwards.
func RunTest(t *gotest.T, timeout time.Duration, run func()) {
	if timeout <= 0 {
		run()
		return
	}
	done := make(chan struct{})
	defer close(done) // also runs when the method exits via Goexit
	go watchDeadline(t, timeout, done)
	run()
}

// watchDeadline fails t if run has not finished within timeout. Errorf is safe
// from another goroutine while the test is still running, which is exactly the
// window this exists for.
func watchDeadline(t *gotest.T, timeout time.Duration, done <-chan struct{}) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
	}

	// Written unbuffered as well as recorded on the test. A test that never
	// returns never completes, and the testing package only flushes a test's
	// output when it does — so if the process is later killed by go test
	// -timeout, this line is the only trace that a budget was blown, and which
	// one.
	fmt.Fprintf(os.Stderr, "gotest: %s exceeded its configured Timeout of %s and is still running\n", t.T().Name(), timeout)
	t.Errorf("exceeded its configured Timeout of %s and is still running", timeout)
}

// reportOverrun fails t when its deadline expired rather than being canceled by
// the work finishing. A context that has already expired keeps DeadlineExceeded,
// so a later cancellation cannot mask it.
func reportOverrun(t *gotest.T, timeout time.Duration, what, budget string) {
	if errors.Is(t.Context().Err(), context.DeadlineExceeded) {
		t.Errorf("%sexceeded its configured %s of %s", what, budget, timeout)
	}
}

// RunSetup runs a suite's BeforeAll and reports an overrun of SetupTimeout.
//
// The check has to happen here rather than in a cleanup: the deadline would also
// have passed by the end of a suite whose BeforeAll was fast but whose tests
// were slow, and that is not an overrun.
func RunSetup(t *testing.T, timeout time.Duration, beforeAll func(*gotest.T)) {
	tt := SetupT(t, timeout)
	beforeAll(tt)
	reportOverrun(tt, timeout, "BeforeAll ", "SetupTimeout")
}

// RunTeardown runs a suite's AfterAll from inside t.Cleanup and reports an
// overrun of SetupTimeout.
func RunTeardown(t *testing.T, timeout time.Duration, afterAll func(*gotest.T)) {
	tt := TeardownT(t, timeout)
	afterAll(tt)
	reportOverrun(tt, timeout, "AfterAll ", "SetupTimeout")
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

// OverlayFixtureConfig merges overlay into base: non-zero fields in overlay
// replace the corresponding base field; zero fields are preserved.
func OverlayFixtureConfig(base *gotest.FixtureConfig, overlay gotest.FixtureConfig) {
	if overlay.Timeout != 0 {
		base.Timeout = overlay.Timeout
	}
	if overlay.Retries != 0 {
		base.Retries = overlay.Retries
	}
	if overlay.RetryDelay != 0 {
		base.RetryDelay = overlay.RetryDelay
	}
}

// OverlaySuiteConfig merges overlay into base: non-zero fields replace the
// corresponding base field. FailFast and Parallel are one-way latches — once
// true, an overlay with false will not reset them.
func OverlaySuiteConfig(base *gotest.SuiteConfig, overlay gotest.SuiteConfig) {
	if overlay.Timeout != 0 {
		base.Timeout = overlay.Timeout
	}
	if overlay.SetupTimeout != 0 {
		base.SetupTimeout = overlay.SetupTimeout
	}
	if overlay.Retries != 0 {
		base.Retries = overlay.Retries
	}
	if overlay.FailFast {
		base.FailFast = true
	}
	if overlay.Parallel {
		base.Parallel = true
	}
}
