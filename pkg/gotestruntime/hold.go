package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"testing"
)

// HoldFixtures sets the fixtures up for tb, a top-level generated test,
// benchmark or fuzz function, and tears them down in tb's cleanup. Generated
// code calls it first, so the teardown runs after every other cleanup of tb,
// the suite's AfterAll among them.
//
// The testing package runs that cleanup when a test or subtest below tb
// panics, too. A panicking sub-benchmark ends as a failure instead: see
// ReleaseOnPanic.
// While the hold lasts, a stop (Ctrl-C, or one the runner sends) ends the
// tests' contexts, skips the methods not started, and lets the suite finish:
// AfterAll runs, then this cleanup releases the fixtures.
func HoldFixtures(tb testing.TB, build func() MainConfig) {
	refuseLegacyRunner(tb)
	// Failures are reported through reportAt: tb.Fatalf would locate them
	// in this file.
	cfg, err := buildConfig(build)
	if err != nil {
		reportAt(tb, "", "fixture setup: "+err.Error())
		tb.FailNow()
		return
	}
	drainLink() // a stop that came while no suite was open sets nothing up
	startWatch()
	dag, err := setupFixtureDAG(context.Background(), cfg, hold)
	if err != nil {
		stopWatch()
		// Not a setup that failed: none was attempted.
		var refused *RunnerRequiredError
		if errors.As(err, &refused) {
			reportAt(tb, "", refused.report())
			tb.FailNow()
			return
		}
		reportAt(tb, failureAt(err), "fixture setup: "+err.Error())
		tb.FailNow()
		return
	}
	tb.Cleanup(func() {
		failed := dag.Teardown()
		unhold(dag)
		stopWatch()
		for _, f := range dag.failures {
			reportAt(tb, f.where(), fmt.Sprintf("fixture teardown failed: %s.%s: %v", f.fixture, f.hook, f.err))
		}
		if failed && len(dag.failures) == 0 {
			reportAt(tb, "", "fixture teardown failed")
		}
	})
}

// buildConfig runs build, turning a panic, such as one in a config method,
// into an error.
func buildConfig(build func() MainConfig) (cfg MainConfig, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n\n%s", r, debug.Stack())
		}
	}()
	return build(), nil
}

// The fixtures held right now: at most one DAG per test package of the
// binary, since generated top-level functions never run in parallel.
var (
	heldMu sync.Mutex
	held   = map[*FixtureDAG]struct{}{}
)

func hold(dag *FixtureDAG) {
	heldMu.Lock()
	held[dag] = struct{}{}
	heldMu.Unlock()
}

func unhold(dag *FixtureDAG) {
	heldMu.Lock()
	delete(held, dag)
	heldMu.Unlock()
}

// releaseHeld tears down every held DAG, waiting for a teardown already
// under way. The hold's own cleanup drops it.
func releaseHeld() {
	heldMu.Lock()
	dags := make([]*FixtureDAG, 0, len(held))
	for dag := range held {
		dags = append(dags, dag)
	}
	heldMu.Unlock()
	for _, dag := range dags {
		dag.Teardown()
	}
}

// ReleaseOnPanic ends a sub-benchmark that panics as a failed benchmark. It
// recovers the panic, prints it with the stack from where it happened, and
// stops the sub-benchmark; the parent's cleanups, the fixture teardown among
// them, then run as after any failure. Generated code defers it directly in
// each sub-benchmark (the name is from when it only released the fixtures).
//
// A panic left to the testing package can be lost: it signals the benchmark
// done while the panic unwinds, and the process may print PASS and exit 0
// before the panic ends it. Recovered, the benchmark fails every time.
//
// Under GODEBUG=panicnil=1 it does not recover: recover would stop a
// panic(nil) while reporting nil.
func ReleaseOnPanic(b *testing.B) {
	if panicNilLegacy {
		return
	}
	if r := recover(); r != nil {
		reportPanic(b, r)
		b.FailNow()
	}
}

// failOnPanic is [ReleaseOnPanic] for a benchmark's cleanup, which goes on to
// the next cleanup after the failure instead of stopping.
func failOnPanic(b *testing.B) {
	if panicNilLegacy {
		return
	}
	if r := recover(); r != nil {
		reportPanic(b, r)
	}
}

// reportPanic fails b with a recovered panic, printed as the runtime prints
// one, so the traceback starts where the panic did.
func reportPanic(b *testing.B, r any) {
	fmt.Fprintf(os.Stderr, "panic: %v [recovered]\n\n%s\n", r, debug.Stack())
	b.Fail()
}

var panicNilLegacy = probePanicNil()

func probePanicNil() (legacy bool) {
	defer func() { legacy = recover() == nil }()
	panic(nil) //nolint:govet // the probe: what recover reports for panic(nil)
}
