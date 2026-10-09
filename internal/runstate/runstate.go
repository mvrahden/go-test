// Package runstate is the process-wide state of a test run that pkg/gotest and
// pkg/gotestruntime both read: pkg/gotestruntime imports pkg/gotest, so neither
// can hold it for the other.
package runstate

import (
	"context"
	"flag"
	"sync/atomic"
)

// FuzzWorker reports whether this process is a fuzzing worker. Go stops a
// worker itself on an interrupt.
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
	fuzzEngine.Store(false)
	stopCtx, stopCancel = context.WithCancel(context.Background())
}
