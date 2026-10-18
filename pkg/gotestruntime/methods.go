package gotestruntime

import (
	"context"
	"github.com/mvrahden/go-test/internal/runstate"
	"sync/atomic"
	"testing"

	"github.com/mvrahden/go-test/internal/callsite"
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
}

// Method is one test method. Run calls the suite's BeforeEach, the method
// through run, and the suite's AfterEach; only it knows their static types.
type Method struct {
	Name string
	Run  func(t *gotest.T, run func(method func()))
}

// RunMethods runs each method as a subtest of t: fixture BeforeEach hooks,
// then the method's Run, then fixture AfterEach hooks in reverse. cfg bounds
// each method's context and budget is the Timeout it is held to, as returned
// by [OpenSuite]. With cfg.FailFast, a failure stops the methods that have
// not started yet. Once the run is asked to stop, a method that has not
// started is skipped with the reason "run stopping".
func RunMethods(t *testing.T, cfg, budget gotest.SuiteConfig, m Methods) {
	var failed atomic.Bool
	for _, c := range m.Cases {
		t.Run(c.Name, func(it *testing.T) {
			if m.Parallel {
				it.Parallel()
				if cfg.FailFast && failed.Load() {
					it.Skip("FailFast: earlier test failed")
				}
			}
			if runstate.StopContext().Err() != nil {
				it.Skip("run stopping")
			}
			if m.Parallel {
				defer func() {
					if it.Failed() {
						failed.Store(true)
					}
				}()
			}
			tt := TestT(it, cfg.Timeout)
			for _, f := range m.Fixtures {
				if f.After != nil {
					defer func() {
						if err := f.After(context.Background()); err != nil { //nolint:fail-guard // the method has run; report and go on
							it.Errorf("%s.AfterEach failed: %v", f.Name, err)
						}
					}()
				}
			}
			for _, f := range m.Fixtures {
				if f.Before != nil {
					if err := f.Before(it.Context()); err != nil { //nolint:fail-guard // names the fixture and its hook
						it.Fatalf("%s.BeforeEach failed: %v", f.Name, err)
					}
				}
			}
			c.Run(tt, func(method func()) { RunTest(tt, budget.Timeout, method) })
		})
		if !m.Parallel && cfg.FailFast && t.Failed() {
			return
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
	endBody(t.T())
	select {
	case <-done:
		return // called in time, even if the deadline has passed since
	default:
	}
	select {
	case <-done:
	case <-t.Context().Done():
		reportAt(t.T(), callsite.Declared(at), name+": done() was not called before the test deadline")
		t.T().FailNow() //nolint:t-escape // runtime verdict, located at the declaration
	}
}
