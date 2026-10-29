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
	// AfterAll releases what a failed attempt reached before the next attempt
	// runs, so a retry never builds on top of a half-initialised one. Nil
	// releases nothing between attempts.
	AfterAll func(ctx context.Context) error
	// Held, if set, reports whether a failed attempt may still hold what it
	// reached: true just before each attempt's BeforeAll, false once the
	// release between attempts has run. The caller's teardown releases a
	// fixture last reported held, so every BeforeAll entered is released
	// once. A setup cancelled before the first attempt never reports it.
	Held func(held bool)
}

// ErrSetupOverran reports a BeforeAll that returned success only after its
// declared budget had expired. It is terminal — never retried, because the
// overrun is deterministic and the work is already done — and callers use it
// to keep the fixture eligible for teardown: the resources it created exist
// and must be released even though the run is failing.
var ErrSetupOverran = errors.New("exceeded its configured Timeout")

// ErrReleaseFailed reports an AfterAll between attempts that failed. The setup
// fails with it even when a later attempt succeeds: what the failed attempt
// reached may still be held.
var ErrReleaseFailed = errors.New("AfterAll between attempts failed")

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
//
// An AfterAll between attempts that fails does not stop the retry, but the
// setup then fails with ErrReleaseFailed however its attempts end. The
// release is not cut short by a cancelled ctx, only by the fixture's Timeout,
// as in the final teardown; a cancel seen once it returns ends the setup
// without another attempt.
func RunFixtureSetup(ctx context.Context, s FixtureSetup) error {
	if s.BeforeAll == nil {
		return nil
	}
	attempts := 1 + s.Retries
	var lastErr, releaseErr error

	for i := range attempts {
		if i == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		if s.Held != nil {
			s.Held(true)
		}

		var attemptCtx context.Context
		var attemptCancel context.CancelFunc
		if s.Timeout > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(ctx, s.Timeout)
		} else {
			attemptCtx, attemptCancel = context.WithCancel(ctx)
		}

		lastErr = attemptSetup(s.BeforeAll, attemptCtx)
		overran := lastErr == nil && s.Budget > 0 && ctx.Err() == nil &&
			errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
		attemptCancel()

		if overran {
			// Setup that ignores the context still overran the budget it was
			// given; without this a declared Timeout would mean nothing. Only a
			// declared one, though — failing a fixture against a default it
			// never asked for is not a verdict its author could act on. And only
			// when the attempt's own deadline fired: a parent that expired
			// mid-attempt is the caller's deadline, not this fixture's budget.
			//
			// The verdict is terminal, not retried: the attempt succeeded, so a
			// retry would create a second set of whatever it built, and the
			// overrun would repeat deterministically.
			err := fmt.Errorf("%w of %s", ErrSetupOverran, scaleTimeout(s.Budget))
			fmt.Fprintf(os.Stderr, "FAIL: %s.BeforeAll %v\n", s.Name, err)
			return errors.Join(err, releaseErr)
		}

		if lastErr == nil {
			if releaseErr != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %s.BeforeAll succeeded on attempt %d/%d, but %v\n", s.Name, i+1, attempts, releaseErr)
			}
			return releaseErr
		}
		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), releaseErr)
		}
		if i < attempts-1 {
			fmt.Fprintf(os.Stderr, "%s.BeforeAll attempt %d/%d failed: %v\n", s.Name, i+1, attempts, lastErr)
			// The delay runs before the release: a parent cancelled meanwhile
			// returns with the attempt still held, and the caller's teardown
			// releases it once, as it does any attempted fixture.
			if s.RetryDelay > 0 {
				select {
				case <-ctx.Done():
					return errors.Join(ctx.Err(), releaseErr)
				case <-time.After(s.RetryDelay):
				}
			}
			if err := release(ctx, s); err != nil {
				// The retry runs anyway, but the setup fails however it ends.
				releaseErr = errors.Join(releaseErr, err)
			}
			if s.Held != nil {
				s.Held(false)
			}
			if ctx.Err() != nil {
				return errors.Join(ctx.Err(), releaseErr)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "FAIL: %s.BeforeAll failed after %d attempt(s): %v\n", s.Name, attempts, lastErr)
	if releaseErr != nil {
		return errors.Join(lastErr, releaseErr)
	}
	return lastErr
}

// release runs AfterAll between attempts and returns ErrReleaseFailed, with
// what failed, if the release failed. It runs
// as the final teardown does: under the fixture's Timeout and held to its
// declared one, and not cut short by a cancel of ctx.
func release(ctx context.Context, s FixtureSetup) error {
	if s.AfterAll == nil {
		return nil
	}
	td := FixtureTeardown{Name: s.Name, Timeout: s.Timeout, Budget: s.Budget, AfterAll: s.AfterAll}
	if err := runFixtureTeardown(context.WithoutCancel(ctx), td); err != nil {
		return fmt.Errorf("%w: %w", ErrReleaseFailed, err)
	}
	return nil
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
