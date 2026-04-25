package assert

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type formatF func(format string, args ...any)

type BaseAssertionContext struct {
	v    any
	t    *testing.T
	fail formatF
}

func NewAssertionContext(v any, t *testing.T) *BaseAssertionContext {
	return &BaseAssertionContext{
		v: v,
		t: t,
		fail: func(format string, args ...any) {
			t.Helper()
			msg := fmt.Sprintf(format, args...)
			if trace := CallerTrace(); trace != "" {
				msg += trace
			}
			t.Fatalf("%s", msg)
		},
	}
}

func NewAssertionContextForTest(v any, fail formatF) *BaseAssertionContext {
	return &BaseAssertionContext{v: v, fail: fail}
}

// Equal delegates to CheckEqual(expected, b.v).
func (b *BaseAssertionContext) Equal(expected any) {
	if b.t != nil {
		b.t.Helper()
	}
	msg := CheckEqual(expected, b.v)
	if msg != "" {
		b.fail("%s", msg)
	}
}

// IsTrue checks that b.v is bool true.
func (b *BaseAssertionContext) IsTrue() {
	if b.t != nil {
		b.t.Helper()
	}
	v, ok := b.v.(bool)
	if !ok || !v {
		b.fail("True failed:\n  got: %v", b.v)
	}
}

// IsFalse checks that b.v is bool false.
func (b *BaseAssertionContext) IsFalse() {
	if b.t != nil {
		b.t.Helper()
	}
	v, ok := b.v.(bool)
	if !ok || v {
		b.fail("False failed:\n  got: %v", b.v)
	}
}

// IsZero checks that b.v is the zero value for its type.
// nil (invalid reflect.Value) is treated as an error, not as zero.
func (b *BaseAssertionContext) IsZero() {
	if b.t != nil {
		b.t.Helper()
	}
	rVal := reflect.ValueOf(b.v)
	if !rVal.IsValid() {
		b.fail("Zero failed:\n  nil is not a typed zero value")
		return
	}
	if !rVal.IsZero() {
		b.fail("Zero failed:\n  got: %s", FormatValue(b.v))
	}
}

// IsNotZero checks that b.v is not the zero value for its type.
func (b *BaseAssertionContext) IsNotZero() {
	if b.t != nil {
		b.t.Helper()
	}
	rVal := reflect.ValueOf(b.v)
	if !rVal.IsValid() {
		b.fail("NotZero failed:\n  nil is not a typed zero value")
		return
	}
	if rVal.IsZero() {
		b.fail("NotZero failed:\n  got: %s", FormatValue(b.v))
	}
}

// IsEmpty checks that b.v has length 0 (string, slice, map, array, chan),
// or is nil. Pointer types are recursively dereferenced.
func (b *BaseAssertionContext) IsEmpty() {
	if b.t != nil {
		b.t.Helper()
	}
	if !checkIsEmpty(b.v) {
		rVal := derefPtr(reflect.ValueOf(b.v))
		switch k := rVal.Kind(); k {
		case reflect.String, reflect.Map, reflect.Array, reflect.Slice, reflect.Chan:
			b.fail("Empty failed:\n  got length: %d", rVal.Len())
		default:
			b.fail("Empty failed:\n  value of type <%s> cannot be empty", k.String())
		}
	}
}

// HasLength asserts that b.v has the given length.
// Supports String/Map/Array/Slice/Chan; dereferences pointers.
func (b *BaseAssertionContext) HasLength(l int) {
	if b.t != nil {
		b.t.Helper()
	}
	rVal := derefPtr(reflect.ValueOf(b.v))
	switch k := rVal.Kind(); k {
	case reflect.String, reflect.Map, reflect.Array, reflect.Slice, reflect.Chan:
		if actual := rVal.Len(); actual != l {
			b.fail("HasLength failed:\n  expected length: %d\n  actual length:   %d", l, actual)
		}
	default:
		b.fail("HasLength failed:\n  value of type <%s> does not have a length", k.String())
	}
}

// HasCapacity asserts that b.v has the given capacity.
// Supports Array/Slice/Chan; dereferences pointers.
func (b *BaseAssertionContext) HasCapacity(c int) {
	if b.t != nil {
		b.t.Helper()
	}
	rVal := derefPtr(reflect.ValueOf(b.v))
	switch k := rVal.Kind(); k {
	case reflect.Array, reflect.Slice, reflect.Chan:
		if actual := rVal.Cap(); actual != c {
			b.fail("HasCapacity failed:\n  expected capacity: %d\n  actual capacity:   %d", c, actual)
		}
	default:
		b.fail("HasCapacity failed:\n  value of type <%s> does not have a capacity", k.String())
	}
}

// Contains checks that b.v contains v.
// For strings: substring search. For slices/arrays: element search via reflect.DeepEqual.
func (b *BaseAssertionContext) Contains(v any) {
	if b.t != nil {
		b.t.Helper()
	}
	rVal := reflect.ValueOf(b.v)
	switch k := rVal.Kind(); k {
	case reflect.String:
		substr, ok := v.(string)
		if !ok {
			b.fail("Contains failed:\n  string container requires string element, got %T", v)
			return
		}
		if !strings.Contains(rVal.String(), substr) {
			b.fail("Contains failed:\n  %q does not contain %q", b.v, substr)
		}
	case reflect.Array, reflect.Slice:
		vVal := reflect.ValueOf(v)
		for i := range rVal.Len() {
			if reflect.DeepEqual(rVal.Index(i).Interface(), vVal.Interface()) {
				return
			}
		}
		b.fail("Contains failed:\n  %v does not contain %v", b.v, v)
	default:
		b.fail("Contains failed:\n  value of type <%s> does not support Contains", k.String())
	}
}

// NoError checks that b.v is nil or is an error interface holding nil.
func (b *BaseAssertionContext) NoError() {
	if b.t != nil {
		b.t.Helper()
	}
	if b.v == nil {
		return
	}
	if err, ok := b.v.(error); ok {
		if err == nil {
			return
		}
		b.fail("NoError failed:\n  got: %v", err)
		return
	}
	b.fail("NoError failed:\n  value of type %T is not an error", b.v)
}

// checkIsEmpty returns true if v is considered empty.
// nil → true. string/slice/map/array/chan with Len()==0 → true.
// Pointers are recursively dereferenced; a nil pointer at any level → true.
func checkIsEmpty(v any) bool {
	if v == nil {
		return true
	}
	rVal := reflect.ValueOf(v)
	return checkIsEmptyValue(rVal)
}

func checkIsEmptyValue(rVal reflect.Value) bool {
	switch k := rVal.Kind(); k {
	case reflect.Pointer, reflect.Interface:
		if rVal.IsNil() {
			return true
		}
		return checkIsEmptyValue(rVal.Elem())
	case reflect.String, reflect.Map, reflect.Array, reflect.Slice, reflect.Chan:
		return rVal.Len() == 0
	default:
		return false
	}
}

// derefPtr recursively dereferences non-nil pointers and interfaces.
func derefPtr(r reflect.Value) reflect.Value {
	k := r.Kind()
	if k != reflect.Pointer && k != reflect.Interface {
		return r
	}
	if !r.IsNil() {
		return derefPtr(r.Elem())
	}
	return r
}

// IsEqualTo is a legacy alias for Equal, kept for backwards compatibility.
func (b *BaseAssertionContext) IsEqualTo(v any) {
	if b.t != nil {
		b.t.Helper()
	}
	b.Equal(v)
}
