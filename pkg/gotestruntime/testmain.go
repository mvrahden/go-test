package gotestruntime

import (
	"fmt"
	"io"
	"os"
	"sync"
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

// Main runs the tests, then tears down every fixture DAG the run set up. A
// package with fixture-bound suites that defines its own TestMain must call it
// in place of m.Run:
//
//	func TestMain(m *testing.M) { os.Exit(gotestruntime.Main(m)) }
//
// A teardown failure fails the package: the exit code becomes 1 when the tests
// passed.
func Main(m *testing.M) int {
	return finish(m.Run(), os.Stderr)
}

func finish(code int, stderr io.Writer) int {
	if runTeardowns() && code == 0 {
		fmt.Fprintln(stderr, "FAIL: fixture teardown failed")
		return 1
	}
	return code
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
