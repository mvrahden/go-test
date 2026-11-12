package gotestruntime

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

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
	// BeforeAt and AfterAt locate the hooks (file:line, the file relative
	// to the package directory); a failure is reported there, never inside
	// the runtime's own file. Empty, as from a harness generated before
	// them, reports it unlocated.
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
// turn comes once every BeforeEach before it returned without an error inside
// the budget; its AfterEach runs exactly when its turn came and its own
// BeforeEach, if any, returned without an error, so a hook never releases
// what was never acquired. cfg
// bounds each method's context and budget is the Timeout it is held to, as
// returned by [OpenSuite]; the budget covers the hooks and the body alike, and once a
// hook overran it nothing after that hook starts but the AfterEach hooks.
// With cfg.FailFast, the first method that fails stops the ones that have
// not started: each is skipped with the reason "FailFast: <that method>".
// The decision reads each method's own verdict, so a failure the suite's
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
			t.Run(c.Name, func(it *testing.T) { skipFailFast(it, first) })
			continue
		}
		passed := t.Run(c.Name, func(it *testing.T) {
			if m.Parallel {
				it.Parallel()
				// The first cleanup runs last: it sees the failures the others
				// record, the wait on a gotest.Go goroutine's among them.
				it.Cleanup(func() {
					if it.Failed() {
						failedFirst.CompareAndSwap(nil, &c.Name)
					}
				})
				if first := tripped(); first != "" {
					skipFailFast(it, first)
				}
			}
			if runstate.StopContext().Err() != nil {
				skipStopping(it)
			}
			// The budget covers the fixture hooks, the suite hooks, the body and
			// the waits on the method's gotest.Go goroutines among its cleanups.
			mt, end := openMethod(it, cfg.Timeout, budget.Timeout)
			defer end()
			for _, f := range m.Fixtures {
				// After a BeforeEach that overran no later fixture's turn comes.
				if mt.overran() {
					return
				}
				if f.Before != nil {
					mt.enter(f.Name + ".BeforeEach ")
					if err := f.Before(mt.t.Context()); err != nil { //nolint:fail-guard // names the fixture and its hook, at the hook
						reportAt(it, callsite.Declared(f.BeforeAt), fmt.Sprintf("%s.BeforeEach failed: %v", f.Name, err))
						it.FailNow()
					}
				}
				if f.After != nil {
					defer func() {
						mt.enter(f.Name + ".AfterEach ")
						// Releases what BeforeEach acquired, so a stop does
						// not end its context; a declared Timeout does, the
						// default deadline does not.
						ctx := context.WithoutCancel(mt.t.Context())
						if budget.Timeout > 0 {
							ctx = AfterEachT(mt.t).Context()
						}
						if err := f.After(ctx); err != nil { //nolint:fail-guard // the method has run; report and go on
							reportAt(it, callsite.Declared(f.AfterAt), fmt.Sprintf("%s.AfterEach failed: %v", f.Name, err))
						}
					}()
				}
			}
			if mt.overran() {
				return
			}
			mt.enter("BeforeEach ")
			c.Run(mt.t, func(method func()) {
				if mt.overran() {
					return
				}
				mt.enter("")
				defer runstate.EnterBody(it, "a method")()
				defer sayStopEnded(it)()
				method()
			})
		})
		if !passed {
			failedFirst.CompareAndSwap(nil, &c.Name)
		}
	}
}

// skipFailFast skips t, naming the method that stopped the suite, with no
// location: a runtime file is nothing the user can open.
func skipFailFast(t *testing.T, first string) {
	fmt.Fprintln(t.Output(), "FailFast: "+first)
	t.SkipNow()
}

// stopEndedLine is the line a method body that was running when the stop
// came writes to its output, whether the stop ended it or not; the runner's
// stream reads this exact text.
const stopEndedLine = "run stopping: the stop came while this method ran"

// sayStopEnded returns what to run once a method body returned: when a stop
// came while it ran, the body says so in t's output. A body that began after
// the stop, or one cut short after the grace, never says it.
func sayStopEnded(t *testing.T) func() {
	if runstate.StopContext().Err() != nil {
		return func() {}
	}
	return func() {
		if runstate.StopContext().Err() != nil {
			fmt.Fprintln(t.Output(), stopEndedLine)
		}
	}
}

// skipStopping skips t, a method a stopped run never started, with no
// location either.
func skipStopping(t *testing.T) {
	fmt.Fprintln(t.Output(), "run stopping")
	t.SkipNow()
}
