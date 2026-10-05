// Package runstate is the process-wide state of a test run that pkg/gotest and
// pkg/gotestruntime both read: pkg/gotestruntime imports pkg/gotest, so neither
// can hold it for the other.
package runstate

import (
	"context"
	"flag"
	"sync"
	"sync/atomic"
)

var (
	dying     atomic.Bool
	notedOnce sync.Once
	notedVal  any
	notedAt   []byte
)

// MarkDying records that a panic is leaving a test for the testing package,
// which ends the process after the cleanups, and keeps the first one's value
// and stack to print before fixtures are released.
//
// Call it only where nothing but the testing package can recover the panic. A
// mark set under a recover the user wrote (gotest.Panics) would tear fixtures
// down under a process that keeps running.
func MarkDying(value any, stack []byte) {
	notedOnce.Do(func() { notedVal, notedAt = value, stack })
	dying.Store(true)
}

// Dying reports whether MarkDying was called.
func Dying() bool { return dying.Load() }

// Noted returns the first panic MarkDying recorded.
func Noted() (value any, stack []byte, ok bool) {
	return notedVal, notedAt, notedAt != nil
}

// PanicNilLegacy reports whether this binary runs with GODEBUG=panicnil=1 (or
// a //go:debug line saying so). recover then returns nil for panic(nil) as it
// does for runtime.Goexit, but stops only the former, so a hook that recovers
// to note a panic would swallow panic(nil). Hooks do not recover under it.
var PanicNilLegacy = probePanicNil()

func probePanicNil() (legacy bool) {
	defer func() { legacy = recover() == nil }()
	panic(nil) //nolint:govet // the probe: what recover reports for panic(nil)
}

// FuzzWorker reports whether this process is a fuzzing worker. Go recovers a
// panicking input there and keeps fuzzing, and stops the worker itself on an
// interrupt.
func FuzzWorker() bool {
	f := flag.Lookup("test.fuzzworker")
	return f != nil && f.Value.String() == "true"
}

// FuzzTarget returns the -test.fuzz pattern, or "" when nothing is fuzzed.
func FuzzTarget() string {
	if f := flag.Lookup("test.fuzz"); f != nil {
		return f.Value.String()
	}
	return ""
}

var fuzzEngine atomic.Bool

// SetFuzzEngine records that the fuzz target -test.fuzz selects was handed to
// Go's fuzzing engine, which stops gracefully on an interrupt from then on.
func SetFuzzEngine() { fuzzEngine.Store(true) }

// FuzzEngine reports whether SetFuzzEngine was called.
func FuzzEngine() bool { return fuzzEngine.Load() }

var stopCtx, stopCancel = context.WithCancel(context.Background())

// StopContext is canceled when the run is asked to stop. The contexts gotest
// hands to tests and fixture setup derive from it, so work that honours its
// context ends and releases what it holds before fixtures tear down.
func StopContext() context.Context { return stopCtx }

// Stop cancels StopContext.
func Stop() { stopCancel() }

// Reset clears the state. For tests.
func Reset() {
	dying.Store(false)
	notedOnce = sync.Once{}
	notedVal, notedAt = nil, nil
	fuzzEngine.Store(false)
	stopCtx, stopCancel = context.WithCancel(context.Background())
}
