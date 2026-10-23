// Package carry carries a panic across a boundary the runtime cannot cross on
// its own, for pkg/gotest and pkg/gotestruntime both: pkg/gotestruntime imports
// pkg/gotest, so neither can hold it for the other.
package carry

import (
	"fmt"
	"runtime/debug"
)

// Panic is a recovered panic with the stack from where it happened.
//
// Re-panicking with the original value would report the stack of whatever
// re-raised it, which points at gotest internals rather than at the code that
// actually panicked. The runtime prints a panic value that implements
// fmt.Stringer via String, so the whole thing lands in the output unchanged.
type Panic struct {
	value any
	stack []byte
	where string
}

func (c *Panic) String() string {
	return fmt.Sprintf("%v\n\n[recovered from %s]\n%s", c.value, c.where, c.stack)
}

// Unwrap exposes the original panic value to anything that recovers this.
func (c *Panic) Unwrap() any { return c.value }

// Stack is the stack from where the panic happened.
func (c *Panic) Stack() []byte { return c.stack }

// From converts a recovered value into a Panic. Callers must invoke recover()
// themselves, directly inside the deferred function: recover only reports a
// panic to the function the runtime defers.
//
// It returns nil when there is no panic, which is how callers tell a panic apart
// from a runtime.Goexit: recover reports nil for the latter.
func From(v any, where string) *Panic {
	if v == nil {
		return nil
	}
	if c, ok := v.(*Panic); ok {
		return c // already captured further in; keep the original stack
	}
	return &Panic{value: v, stack: debug.Stack(), where: where}
}
