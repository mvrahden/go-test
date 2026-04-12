package gotest

import (
	"reflect"
	"strings"
)

// AssertContext provides fluent assertion methods for a single value.
type AssertContext struct {
	v     any
	failf func(format string, args ...any)
}

// NewAssertContext creates an AssertContext with a custom failure function.
// This is primarily useful for testing assertions themselves.
func NewAssertContext(v any, failf func(format string, args ...any)) *AssertContext {
	return &AssertContext{v: v, failf: failf}
}

// IsTrue asserts the value is boolean true.
func (a *AssertContext) IsTrue() {
	v, ok := a.v.(bool)
	if !ok || !v {
		a.failf("expected true, got %v", a.v)
	}
}

// IsFalse asserts the value is boolean false.
func (a *AssertContext) IsFalse() {
	v, ok := a.v.(bool)
	if !ok || v {
		a.failf("expected false, got %v", a.v)
	}
}

// Equals asserts the value deeply equals the expected value.
func (a *AssertContext) Equals(expected any) {
	if !reflect.DeepEqual(a.v, expected) {
		a.failf("expected %v (%T), got %v (%T)", expected, expected, a.v, a.v)
	}
}

// IsNil asserts the value is nil.
func (a *AssertContext) IsNil() {
	if !isNil(a.v) {
		a.failf("expected nil, got %v (%T)", a.v, a.v)
	}
}

// IsNotNil asserts the value is not nil.
func (a *AssertContext) IsNotNil() {
	if isNil(a.v) {
		a.failf("expected non-nil, got nil")
	}
}

// IsZero asserts the value is the zero value for its type.
func (a *AssertContext) IsZero() {
	rv := reflect.ValueOf(a.v)
	if !rv.IsValid() {
		return // nil is zero
	}
	if !rv.IsZero() {
		a.failf("expected zero value, got %v", a.v)
	}
}

// HasLength asserts the value (string, slice, map, array, or channel) has the given length.
func (a *AssertContext) HasLength(expected int) {
	rv := reflect.ValueOf(a.v)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
		actual := rv.Len()
		if actual != expected {
			a.failf("expected length %d, got %d", expected, actual)
		}
	default:
		a.failf("HasLength not supported for type %T", a.v)
	}
}

// IsEmpty asserts the value (string, slice, map, array, or channel) has length 0.
func (a *AssertContext) IsEmpty() {
	a.HasLength(0)
}

// Contains asserts the value contains the given element.
// For strings: checks substring containment.
// For slices/arrays: checks element presence via DeepEqual.
func (a *AssertContext) Contains(element any) {
	rv := reflect.ValueOf(a.v)
	switch rv.Kind() {
	case reflect.String:
		s := rv.String()
		sub, ok := element.(string)
		if !ok {
			a.failf("Contains on string requires a string argument, got %T", element)
			return
		}
		if !strings.Contains(s, sub) {
			a.failf("expected %q to contain %q", s, sub)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), element) {
				return
			}
		}
		a.failf("expected %v to contain %v", a.v, element)
	default:
		a.failf("Contains not supported for type %T", a.v)
	}
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}
