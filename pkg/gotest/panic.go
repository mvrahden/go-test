package gotest

import (
	"fmt"
	"runtime/debug"
)

// capturedPanic carries a panic across a boundary the runtime cannot cross on
// its own — a helper goroutine, or a deferred function that has to finish
// coordinating before the panic may continue.
//
// Re-panicking with the original value would report the stack of whatever
// re-raised it, which points at gotest internals rather than at the code that
// actually panicked. Keeping the stack captured at the original site preserves
// that. The runtime prints a panic value that implements fmt.Stringer via
// String, so the whole thing lands in the output unchanged.
type capturedPanic struct {
	value any
	stack []byte
	where string
}

func (c *capturedPanic) String() string {
	return fmt.Sprintf("%v\n\n[recovered from %s]\n%s", c.value, c.where, c.stack)
}

// Unwrap exposes the original panic value to anything that recovers this.
func (c *capturedPanic) Unwrap() any { return c.value }

// capturedFrom converts a recovered value into a capturedPanic. Callers must
// invoke recover() themselves, directly inside the deferred function — recover
// only reports a panic to the function the runtime defers, so wrapping the call
// one level deeper would silently always yield nil.
//
// It returns nil when there is no panic, which is how callers tell a panic apart
// from a runtime.Goexit: recover reports nil for the latter.
func capturedFrom(v any, where string) *capturedPanic {
	if v == nil {
		return nil
	}
	if c, ok := v.(*capturedPanic); ok {
		return c // already captured further in; keep the original stack
	}
	return &capturedPanic{value: v, stack: debug.Stack(), where: where}
}
