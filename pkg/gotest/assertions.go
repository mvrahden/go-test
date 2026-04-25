package gotest

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type testingT interface {
	Helper()
	Errorf(format string, args ...any)
	FailNow()
}

func fail(t testingT, msg string, msgAndArgs []any) {
	t.Helper()
	if userMsg := assert.FormatMessage(msgAndArgs); userMsg != "" {
		msg = msg + "\n  message: " + userMsg
	}
	t.Errorf(msg)
	t.FailNow()
}

// Equal asserts that expected and actual are deeply equal.
func Equal[T any](t testingT, expected, actual T, msgAndArgs ...any) {
	t.Helper()
	if msg := assert.CheckEqual(expected, actual); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// NotEqual asserts that expected and actual are NOT deeply equal.
func NotEqual[T any](t testingT, expected, actual T, msgAndArgs ...any) {
	t.Helper()
	if msg := assert.CheckNotEqual(expected, actual); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// True asserts that value is true.
func True(t testingT, value bool, msgAndArgs ...any) {
	t.Helper()
	if !value {
		fail(t, "True failed:\n  expected: true\n  actual:   false", msgAndArgs)
	}
}

// False asserts that value is false.
func False(t testingT, value bool, msgAndArgs ...any) {
	t.Helper()
	if value {
		fail(t, "False failed:\n  expected: false\n  actual:   true", msgAndArgs)
	}
}

// Zero asserts that value is the zero value for its type.
func Zero[T comparable](t testingT, value T, msgAndArgs ...any) {
	t.Helper()
	var zero T
	if value != zero {
		fail(t, fmt.Sprintf("Zero failed:\n  expected: %#v (zero value)\n  actual:   %#v", zero, value), msgAndArgs)
	}
}

// NotZero asserts that value is NOT the zero value for its type.
func NotZero[T comparable](t testingT, value T, msgAndArgs ...any) {
	t.Helper()
	var zero T
	if value == zero {
		fail(t, fmt.Sprintf("NotZero failed:\n  value is the zero value: %#v", value), msgAndArgs)
	}
}

// isEmpty checks if a value is considered empty (nil, or has Len() == 0).
func isEmpty(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.Chan, reflect.String:
		return rv.Len() == 0
	case reflect.Ptr:
		if rv.IsNil() {
			return true
		}
	}
	return false
}

// Empty asserts that object is empty (nil, or has length 0).
func Empty(t testingT, object any, msgAndArgs ...any) {
	t.Helper()
	if !isEmpty(object) {
		fail(t, fmt.Sprintf("Empty failed:\n  object is not empty: %#v", object), msgAndArgs)
	}
}

// NotEmpty asserts that object is NOT empty.
func NotEmpty(t testingT, object any, msgAndArgs ...any) {
	t.Helper()
	if isEmpty(object) {
		fail(t, fmt.Sprintf("NotEmpty failed:\n  object is empty: %#v", object), msgAndArgs)
	}
}

// NoError asserts that err is nil.
func NoError(t testingT, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		fail(t, fmt.Sprintf("NoError failed:\n  unexpected error: %s", err.Error()), msgAndArgs)
	}
}

// Error asserts that err is not nil.
func Error(t testingT, err error, msgAndArgs ...any) {
	t.Helper()
	if err == nil {
		fail(t, "Error failed:\n  expected an error but got nil", msgAndArgs)
	}
}

// ErrorIs asserts that errors.Is(err, target) is true.
func ErrorIs(t testingT, err, target error, msgAndArgs ...any) {
	t.Helper()
	if !errors.Is(err, target) {
		fail(t, fmt.Sprintf("ErrorIs failed:\n  error: %v\n  target: %v", err, target), msgAndArgs)
	}
}

// ErrorAs asserts that errors.As(err, &target) is true and returns the matched error.
func ErrorAs[E error](t testingT, err error, msgAndArgs ...any) E {
	t.Helper()
	var target E
	if !errors.As(err, &target) {
		fail(t, fmt.Sprintf("ErrorAs failed:\n  error %v does not match expected type", err), msgAndArgs)
	}
	return target
}

// ErrorContains asserts that err is not nil and err.Error() contains the given substring.
func ErrorContains(t testingT, err error, contains string, msgAndArgs ...any) {
	t.Helper()
	if err == nil {
		fail(t, "ErrorContains failed:\n  expected an error but got nil", msgAndArgs)
		return
	}
	if !strings.Contains(err.Error(), contains) {
		fail(t, fmt.Sprintf("ErrorContains failed:\n  error: %q\n  does not contain: %q", err.Error(), contains), msgAndArgs)
	}
}
