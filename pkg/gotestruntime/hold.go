package gotestruntime

import (
	"context"
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
// panics, too. A panicking sub-benchmark is the exception: see ReleaseOnPanic.
func HoldFixtures(tb testing.TB, build func() MainConfig) {
	cfg, err := buildConfig(build)
	var dag *FixtureDAG
	if err == nil {
		dag, err = SetupFixtureDAG(context.Background(), cfg)
	}
	if err != nil {
		tb.Fatalf("fixture setup: %v", err)
		return
	}
	hold(dag)
	tb.Cleanup(func() {
		defer unhold(dag)
		if dag.Teardown() {
			tb.Errorf("fixture teardown failed")
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

// releaseHeld tears down every held DAG.
func releaseHeld() {
	heldMu.Lock()
	dags := make([]*FixtureDAG, 0, len(held))
	for dag := range held {
		dags = append(dags, dag)
	}
	heldMu.Unlock()
	for _, dag := range dags {
		dag.Teardown()
		unhold(dag)
	}
}

// ReleaseOnPanic releases the held fixtures when a panic leaves a
// sub-benchmark, then lets it go on. The testing package runs only the
// panicking benchmark's own cleanups before the process ends, never its
// parent's, so the hold would leak. Generated code defers it directly in each
// sub-benchmark; the traceback still starts where the panic did.
//
// Under GODEBUG=panicnil=1 it does not recover: recover would stop a
// panic(nil) while reporting nil.
func ReleaseOnPanic(b *testing.B) {
	if panicNilLegacy {
		return
	}
	if r := recover(); r != nil {
		fmt.Fprintf(os.Stderr, "gotest: %s panicked; releasing fixtures before the process ends\n", b.Name())
		releaseHeld()
		panic(r)
	}
}

var panicNilLegacy = probePanicNil()

func probePanicNil() (legacy bool) {
	defer func() { legacy = recover() == nil }()
	panic(nil) //nolint:govet // the probe: what recover reports for panic(nil)
}
