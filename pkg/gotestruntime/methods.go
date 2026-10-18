package gotestruntime

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/callsite"
	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// Methods is a suite's method loop as a generated Test wrapper hands it to
// [RunMethods].
type Methods struct {
	// Parallel runs every method as a parallel subtest.
	Parallel bool
	// Fixtures are the per-method hooks of the suite's package fixtures,
	// dependencies first.
	Fixtures []EachHooks
	Cases    []Method
}

// EachHooks are one package fixture's per-method hooks; either is nil when
// the fixture declares none.
type EachHooks struct {
	Name   string
	Before func(context.Context) error
	After  func(context.Context) error
	// BeforeAt and AfterAt locate the hooks (base:line); a failure is
	// reported there, never inside the runtime's own file.
	BeforeAt string
	AfterAt  string
}

// Method is one test method. Run calls the suite's BeforeEach, the method
// through run, and the suite's AfterEach; only it knows their static types.
type Method struct {
	Name string
	Run  func(t *gotest.T, run func(method func()))
}

// RunMethods runs each method as a subtest of t: fixture BeforeEach hooks,
// then the method's Run, then fixture AfterEach hooks in reverse. A fixture's
// AfterEach pairs with its BeforeEach: it is registered only once that
// succeeded, so a hook never releases what was never acquired. cfg bounds
// each method's context and budget is the Timeout it is held to, as returned
// by [OpenSuite]; the budget covers the hooks and the body alike. With
// cfg.FailFast, the first method that fails stops the ones that have not
// started: each is skipped with the reason "FailFast: <that method>". The
// decision reads each method's own verdict, so a failure the suite's
// BeforeAll recorded without stopping does not trip it. Among parallel
// methods the stop is best effort: methods already running finish. Once the
// run is asked to stop, a method that has not started is skipped with the
// reason "run stopping".
func RunMethods(t *testing.T, cfg, budget gotest.SuiteConfig, m Methods) {
	var failedFirst atomic.Pointer[string]
	tripped := func() string {
		if first := failedFirst.Load(); cfg.FailFast && first != nil {
			return *first
		}
		return ""
	}
	for _, c := range m.Cases {
		if first := tripped(); first != "" {
			t.Run(c.Name, func(it *testing.T) { it.Skipf("FailFast: %s", first) })
			continue
		}
		passed := t.Run(c.Name, func(it *testing.T) {
			if m.Parallel {
				it.Parallel()
				if first := tripped(); first != "" {
					it.Skipf("FailFast: %s", first)
				}
			}
			if runstate.StopContext().Err() != nil {
				it.Skip("run stopping")
			}
			if m.Parallel {
				defer func() {
					if it.Failed() {
						failedFirst.CompareAndSwap(nil, &c.Name)
					}
				}()
			}
			tt := TestT(it, cfg.Timeout)
			// The budget covers the fixture hooks, the suite hooks and the body:
			// ended last, after every AfterEach registered below.
			defer watchMethod(it, budget.Timeout)()
			for _, f := range m.Fixtures {
				if f.Before != nil {
					if err := f.Before(it.Context()); err != nil { //nolint:fail-guard // names the fixture and its hook, at the hook
						reportAt(it, callsite.Declared(f.BeforeAt), fmt.Sprintf("%s.BeforeEach failed: %v", f.Name, err))
						it.FailNow()
					}
				}
				if f.After != nil {
					defer func() {
						if err := f.After(context.Background()); err != nil { //nolint:fail-guard // the method has run; report and go on
							reportAt(it, callsite.Declared(f.AfterAt), fmt.Sprintf("%s.AfterEach failed: %v", f.Name, err))
						}
					}()
				}
			}
			c.Run(tt, func(method func()) { RunTest(tt, budget.Timeout, method) })
		})
		if !passed {
			failedFirst.CompareAndSwap(nil, &c.Name)
		}
	}
}

// AwaitDoneAt runs an async method through start and returns once it calls
// done, failing t when t's context ends first. at is the method's
// declaration (base:line), the verdict's location: the method has returned,
// so no frame of the user's is left to name.
//
// Once start returns, the method's budget watch ends: from then on the
// method is waiting for done, and the context's deadline is its verdict.
func AwaitDoneAt(t *gotest.T, name, at string, start func(done func())) {
	done := make(chan struct{}, 1)
	start(func() {
		select {
		case done <- struct{}{}:
		default:
		}
	})
	budget := endBody(t.T())
	select {
	case <-done:
		return // called in time, even if the deadline has passed since
	default:
	}
	// Only a budget the suite declared is a verdict; without one the context's
	// default deadline bounds the method's work, not the wait for done.
	var deadline <-chan struct{}
	if budget > 0 {
		deadline = t.Context().Done()
	}
	// The failure of the goroutine that owns done leaves nothing to wait for.
	failed := time.NewTicker(failedPoll)
	defer failed.Stop()
	for {
		select {
		case <-done:
			return
		case <-runstate.StopContext().Done():
			t.T().Skip("run stopping") //nolint:t-escape // runtime verdict: the run was stopped
		case <-deadline:
			if runstate.StopContext().Err() != nil {
				t.T().Skip("run stopping") //nolint:t-escape // the stop ended the context, not the deadline
			}
			reportAt(t.T(), callsite.Declared(at), name+": done() was not called before the test deadline")
			t.T().FailNow() //nolint:t-escape // runtime verdict, located at the declaration
		case <-failed.C:
			if t.T().Failed() {
				return
			}
		}
	}
}

// failedPoll is how often an async wait looks for a failure recorded by
// another goroutine; testing has no signal for one.
const failedPoll = 10 * time.Millisecond
