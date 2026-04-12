package gotest

import "testing"

// T wraps *testing.T with suite-aware functionality.
type T struct {
	t *testing.T
}

// NewT creates a T from a *testing.T.
func NewT(t *testing.T) *T {
	return &T{t: t}
}

// T returns the underlying *testing.T.
func (t *T) T() *testing.T { return t.t }

// It runs a named subtest within the current test context.
func (t *T) It(description string, fn func(*T)) {
	t.t.Run(description, func(sub *testing.T) {
		fn(NewT(sub))
	})
}
