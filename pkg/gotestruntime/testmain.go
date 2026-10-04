package gotestruntime

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mvrahden/go-test/internal/dying"
)

var (
	teardownMu sync.Mutex
	teardowns  []func() (failed bool)
	// teardownRun serializes runs, so a stop that arrives while Main tears
	// down waits for it instead of ending the process halfway.
	teardownRun sync.Mutex
)

// RegisterTeardown queues a fixture DAG's teardown for [Main]. Generated code
// calls it once its DAG came up; a DAG that never came up has nothing to
// release.
func RegisterTeardown(fn func() (failed bool)) {
	teardownMu.Lock()
	teardowns = append(teardowns, fn)
	teardownMu.Unlock()
	watchStop()
}

// Run states of the test binary, as fixture setup sees them.
const (
	runNotStarted int32 = iota // m.Run was called directly, or not yet
	runActive                  // the tests run through Main or M
	runFinished                // Main or M tore the fixtures down
)

var runState atomic.Int32

// Main runs the tests, then tears down every fixture DAG the run set up. The
// TestMain gotest generates calls it; a TestMain of your own in a package
// whose suites bind fixtures calls it in place of m.Run:
//
//	func TestMain(m *testing.M) { os.Exit(gotestruntime.Main(m)) }
//
// A teardown failure fails the package: the exit code becomes 1 when the tests
// passed.
func Main(m *testing.M) int {
	return runTests(m.Run, os.Stderr)
}

// M wraps m for a library that runs the tests itself through Run, such as
// goleak.VerifyTestMain or testscript.Main. Its Run runs the tests and then
// tears the fixtures down, as [Main] does:
//
//	func TestMain(m *testing.M) { goleak.VerifyTestMain(gotestruntime.M(m)) }
func M(m *testing.M) interface{ Run() int } {
	return wrappedM{m}
}

type wrappedM struct{ m *testing.M }

func (w wrappedM) Run() int { return runTests(w.m.Run, os.Stderr) }

func runTests(run func() int, stderr io.Writer) int {
	runState.Store(runActive)
	code := run()
	runState.Store(runFinished)
	return finish(code, stderr)
}

func finish(code int, stderr io.Writer) int {
	if runTeardowns() && code == 0 {
		fmt.Fprintln(stderr, "FAIL: fixture teardown failed")
		return 1
	}
	return code
}

// RequireMain reports why fixtures cannot be set up now, or nil. Generated
// setup calls it before every fixture-bound test, benchmark or fuzz target:
// fixtures tear down when Main or M finishes, so tests that did not run
// through either would leave them up, and tests that run after it would find
// them released.
func RequireMain() error {
	switch runState.Load() {
	case runActive:
		return nil
	case runFinished:
		return errors.New("fixtures were already torn down: the tests ran again after gotestruntime.Main or gotestruntime.M returned; run them once, through either")
	default:
		return errors.New("fixtures tear down after the tests only when they run through gotestruntime: in TestMain, call os.Exit(gotestruntime.Main(m)) in place of os.Exit(m.Run()), or pass gotestruntime.M(m) to a library that takes m")
	}
}

// runTeardowns runs every registered teardown once, in registration order, and
// reports whether any failed. Teardowns registered while it runs wait for the
// next call.
func runTeardowns() (failed bool) {
	teardownRun.Lock()
	defer teardownRun.Unlock()
	teardownMu.Lock()
	fns := teardowns
	teardowns = nil
	teardownMu.Unlock()
	for _, fn := range fns {
		if fn() {
			failed = true
		}
	}
	return failed
}

// NotePanic marks the process as dying when a panic passes, then lets it go on.
// Generated code defers it first in every function the testing package calls,
// so it runs last and sees panics from every other deferred call as well.
//
// It must be deferred directly: recover only reports a panic to the function
// the runtime defers.
func NotePanic() {
	if r := recover(); r != nil {
		dying.Mark()
		panic(r)
	}
}

// TeardownIfDying tears the registered fixture DAGs down when a test is
// panicking. The testing package runs cleanups on that path and then ends the
// process, so [Main] never gets to. Registered as a cleanup at fixture setup,
// before the suite's own, it runs after the suite's AfterAll.
func TeardownIfDying() {
	if dying.Marked() {
		runTeardowns()
	}
}
