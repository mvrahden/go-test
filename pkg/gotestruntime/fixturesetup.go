package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureSetup is one fixture's setup policy: how long an attempt may take,
// what it is held to by verdict, and how often to retry.
//
// Both the in-process fixture DAG and the generated shared-fixture subprocess
// run BeforeAll through this. They used to carry separate copies of the loop,
// and only one of them ever learned to contain a panic.
type FixtureSetup struct {
	Name string
	// Timeout bounds each attempt's context. Zero means unbounded.
	Timeout time.Duration
	// Budget is the deadline an attempt is held to by verdict, or zero when the
	// fixture declared no config of its own.
	Budget     time.Duration
	Retries    int
	RetryDelay time.Duration
	// BeforeAll is the setup to run. A nil BeforeAll is "there is nothing to set
	// up" and reports success — callers that populate a FixtureSetup by hand and
	// leave this out get a pass, not a failure.
	BeforeAll func(ctx context.Context) error
}

// RunFixtureSetup runs BeforeAll under s's timeout and retry policy and returns
// the last error, unwrapped — callers add their own context. A nil BeforeAll is
// nothing to do and reports success.
//
// A panic is contained rather than allowed to escape. In the test process it
// would abort a goroutine the testing package knows nothing about; in the
// shared-fixture subprocess it would kill a process holding every other
// fixture's resources, so none of their AfterAlls would ever run. It is also
// the same class of failure Retries exists for — a half-initialised client, an
// empty response indexed into — so it is retried like a returned error.
func RunFixtureSetup(ctx context.Context, s FixtureSetup) error {
	if s.BeforeAll == nil {
		return nil
	}
	attempts := 1 + s.Retries
	var lastErr error

	for i := range attempts {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var attemptCtx context.Context
		var attemptCancel context.CancelFunc
		if s.Timeout > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(ctx, s.Timeout)
		} else {
			attemptCtx, attemptCancel = context.WithCancel(ctx)
		}

		lastErr = attemptSetup(s.BeforeAll, attemptCtx)
		if lastErr == nil && s.Budget > 0 && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			// Setup that ignores the context still overran the budget it was
			// given; without this a declared Timeout would mean nothing. Only a
			// declared one, though — failing a fixture against a default it
			// never asked for is not a verdict its author could act on.
			lastErr = fmt.Errorf("exceeded its configured Timeout of %s", s.Budget)
		}
		attemptCancel()

		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i < attempts-1 {
			fmt.Fprintf(os.Stderr, "%s.BeforeAll attempt %d/%d failed: %v\n", s.Name, i+1, attempts, lastErr)
			if s.RetryDelay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(s.RetryDelay):
				}
			}
		}
	}

	fmt.Fprintf(os.Stderr, "FAIL: %s.BeforeAll failed after %d attempt(s): %v\n", s.Name, attempts, lastErr)
	return lastErr
}

// attemptSetup runs one attempt, turning a panic into an error. debug.Stack()
// here still shows the frames below the panic site, because the deferred
// function runs before the stack unwinds past them.
func attemptSetup(beforeAll func(context.Context) error, ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n\n%s", r, debug.Stack())
		}
	}()
	return beforeAll(ctx)
}

// DeriveFixtureConfig calls a fixture's config marker method with a panic
// contained as an error attributed to the fixture. The generated shared-fixture
// subprocess used to call the marker bare at the top of main, where a panic
// killed the process before the handshake and surfaced as a generic setup
// failure attributed to nothing.
func DeriveFixtureConfig(name string, derive func() gotest.FixtureConfig) (cfg gotest.FixtureConfig, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("deriving its config panicked: %v\n\n%s", r, debug.Stack())
			fmt.Fprintf(os.Stderr, "FAIL: %s: %v\n", name, err)
		}
	}()
	return derive(), nil
}
