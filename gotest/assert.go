package gotest

import (
	"fmt"
	"reflect"

	"github.com/mvrahden/go-test/gotest/internal/require"
)

// AssertContext provides fluent assertion methods for a single value.
// Assertion failures stop the test immediately (via FailNow).
type AssertContext struct {
	v any
	t TestingT
}

// NewAssertContext creates an AssertContext with a custom TestingT.
// Useful for testing assertions themselves.
func NewAssertContext(v any, t TestingT) *AssertContext {
	return &AssertContext{v: v, t: t}
}

func (a *AssertContext) fail(msg string, msgAndArgs ...any) {
	a.t.Helper()
	if len(msgAndArgs) > 0 {
		if fmtStr, ok := msgAndArgs[0].(string); ok {
			msg += "\n\tMessage: " + fmt.Sprintf(fmtStr, msgAndArgs[1:]...)
		}
	}
	a.t.Errorf(msg)
	a.t.FailNow()
}

// Equal checks that the value deeply equals expected.
func (a *AssertContext) Equal(expected any, msgAndArgs ...any) {
	a.t.Helper()
	require.Equal(a.t, expected, a.v, msgAndArgs...)
}

// NotEqual checks that the value does not deeply equal expected.
func (a *AssertContext) NotEqual(expected any, msgAndArgs ...any) {
	a.t.Helper()
	require.NotEqual(a.t, expected, a.v, msgAndArgs...)
}

// IsTrue checks that the value is boolean true.
func (a *AssertContext) IsTrue(msgAndArgs ...any) {
	a.t.Helper()
	v, ok := a.v.(bool)
	if !ok {
		a.fail(fmt.Sprintf("IsTrue requires a bool value, got %T", a.v), msgAndArgs...)
		return
	}
	require.True(a.t, v, msgAndArgs...)
}

// IsFalse checks that the value is boolean false.
func (a *AssertContext) IsFalse(msgAndArgs ...any) {
	a.t.Helper()
	v, ok := a.v.(bool)
	if !ok {
		a.fail(fmt.Sprintf("IsFalse requires a bool value, got %T", a.v), msgAndArgs...)
		return
	}
	require.False(a.t, v, msgAndArgs...)
}

// IsZero checks that the value is the zero value for its type.
func (a *AssertContext) IsZero(msgAndArgs ...any) {
	a.t.Helper()
	rv := reflect.ValueOf(a.v)
	if rv.IsValid() && !rv.IsZero() {
		a.fail(fmt.Sprintf("Should be zero value, got: %v (%T)", a.v, a.v), msgAndArgs...)
	}
}

// IsNotZero checks that the value is not the zero value for its type.
func (a *AssertContext) IsNotZero(msgAndArgs ...any) {
	a.t.Helper()
	rv := reflect.ValueOf(a.v)
	if !rv.IsValid() || rv.IsZero() {
		a.fail(fmt.Sprintf("Should not be zero value, got: %v (%T)", a.v, a.v), msgAndArgs...)
	}
}

// NoError checks that the value is a nil error.
func (a *AssertContext) NoError(msgAndArgs ...any) {
	a.t.Helper()
	if a.v == nil {
		return
	}
	err, ok := a.v.(error)
	if !ok {
		a.fail(fmt.Sprintf("NoError requires an error value, got %T", a.v), msgAndArgs...)
		return
	}
	require.NoError(a.t, err, msgAndArgs...)
}

// IsError checks that the value is a non-nil error.
func (a *AssertContext) IsError(msgAndArgs ...any) {
	a.t.Helper()
	err, ok := a.v.(error)
	if !ok {
		a.fail(fmt.Sprintf("IsError requires an error value, got %T", a.v), msgAndArgs...)
		return
	}
	require.Error(a.t, err, msgAndArgs...)
}

// Empty checks that the value has zero length or is nil.
func (a *AssertContext) Empty(msgAndArgs ...any) {
	a.t.Helper()
	require.Empty(a.t, a.v, msgAndArgs...)
}

// NotEmpty checks that the value has non-zero length.
func (a *AssertContext) NotEmpty(msgAndArgs ...any) {
	a.t.Helper()
	require.NotEmpty(a.t, a.v, msgAndArgs...)
}

// Contains checks that the value contains the element or substring.
func (a *AssertContext) Contains(element any, msgAndArgs ...any) {
	a.t.Helper()
	require.Contains(a.t, a.v, element, msgAndArgs...)
}

// NotContains checks that the value does not contain the element or substring.
func (a *AssertContext) NotContains(element any, msgAndArgs ...any) {
	a.t.Helper()
	require.NotContains(a.t, a.v, element, msgAndArgs...)
}

// HasLength checks that the value has the specified length.
func (a *AssertContext) HasLength(expected int, msgAndArgs ...any) {
	a.t.Helper()
	require.Len(a.t, a.v, expected, msgAndArgs...)
}
