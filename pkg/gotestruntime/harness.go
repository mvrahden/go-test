package gotestruntime

import (
	"context"
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
func TestT(t *testing.T, timeout time.Duration) *gotest.T {
	return testScopedT(t, timeout)
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
