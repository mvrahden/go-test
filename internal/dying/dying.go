// Package dying records that a test panic is on its way out of the process.
//
// Go's testing package runs a panicking test's cleanups and then re-raises the
// panic, so m.Run never returns and anything scheduled after it never runs.
// Fixture teardown therefore has to happen in those cleanups, and they need to
// know the process is dying rather than a test merely failing.
package dying

import "sync/atomic"

var marked atomic.Bool

// Mark records that a panic is leaving a test for the testing package.
//
// Call it only where nothing but the testing package can recover the panic. A
// mark set under a recover the user wrote (gotest.Panics) would tear fixtures
// down under a process that keeps running.
func Mark() { marked.Store(true) }

// Marked reports whether Mark was called.
func Marked() bool { return marked.Load() }

// Reset clears the mark. For tests.
func Reset() { marked.Store(false) }
