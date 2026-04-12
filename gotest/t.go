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

// Assert returns an AssertContext for fluent assertions on the given value.
// Assertion failures stop the test immediately.
func (t *T) Assert(v any) *AssertContext {
	t.t.Helper()
	return &AssertContext{v: v, t: t}
}

// Helper marks the calling function as a test helper function.
func (t *T) Helper() { t.t.Helper() }

// Errorf formats and records a test failure.
func (t *T) Errorf(format string, args ...any) { t.t.Helper(); t.t.Errorf(format, args...) }

// FailNow marks the test as failed and stops its execution.
func (t *T) FailNow() { t.t.FailNow() }

// It runs a named subtest within the current test context.
func (t *T) It(description string, fn func(*T)) {
	t.t.Run(description, func(sub *testing.T) {
		fn(NewT(sub))
	})
}
