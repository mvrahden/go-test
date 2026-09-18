package assert

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// The predicates decide what every assertion that asks counts as nil, empty,
// contained, panicking, a length or equal JSON. They live in the kernel so
// ring-0 tests check them without routing through the assertions built on them.

// DidPanic runs f and reports whether it panicked, with the recovered value.
func DidPanic(f func()) (recovered any, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			recovered = r
			panicked = true
		}
	}()
	f()
	return nil, false
}

// IncludesElement reports whether s contains element: a substring of a
// string, an element of a slice or array (deep equality), or a key of a map.
// valid is false when s is none of those.
func IncludesElement(s, element any) (found, valid bool) {
	if s == nil {
		return false, true
	}
	if str, ok := s.(string); ok {
		sub, ok := element.(string)
		if !ok {
			return false, true
		}
		return strings.Contains(str, sub), true
	}
	rv := reflect.ValueOf(s)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), element) {
				return true, true
			}
		}
		return false, true
	case reflect.Map:
		mapKey := reflect.ValueOf(element)
		if !mapKey.IsValid() || !mapKey.Type().AssignableTo(rv.Type().Key()) {
			return false, true
		}
		return rv.MapIndex(mapKey).IsValid(), true
	}
	return false, false
}

// IsNil reports whether object is nil, for the kinds that can be.
func IsNil(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	}
	return false
}

// IsNilable reports whether object is of a kind that can be nil.
func IsNilable(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return true
	}
	return false
}

// IsEmptyable reports whether object is of a kind that can be empty.
func IsEmptyable(object any) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array,
		reflect.Chan, reflect.String, reflect.Ptr:
		return true
	}
	return false
}

// IsEmpty reports whether a value is considered empty: nil, zero-length
// (slices, maps, arrays, channels, strings), or a pointer to an empty value.
func IsEmpty(object any) bool {
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
		return IsEmpty(rv.Elem().Interface())
	}
	return false
}

// ReadAndRestore reads r to its end and, when r can seek, rewinds it so a
// later reader sees the same bytes.
func ReadAndRestore(r io.Reader) ([]byte, error) {
	if seeker, ok := r.(io.Seeker); ok {
		pos, err := seeker.Seek(0, io.SeekCurrent)
		if err == nil {
			defer func() { _, _ = seeker.Seek(pos, io.SeekStart) }()
		}
	}
	return io.ReadAll(r)
}

// NormalizeJSON converts a value to a JSON-comparable form.
// Accepts string, []byte, json.RawMessage, io.Reader, or any JSON-marshalable value.
func NormalizeJSON(v any) (any, error) {
	var raw []byte
	switch val := v.(type) {
	case string:
		raw = []byte(val)
	case []byte:
		raw = val
	case json.RawMessage:
		raw = []byte(val)
	case io.Reader:
		b, err := ReadAndRestore(val)
		if err != nil {
			return nil, fmt.Errorf("failed to read value: %w", err)
		}
		raw = b
	default:
		var err error
		raw, err = json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %w", err)
		}
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return out, nil
}

// Length reports the length of a slice, array, map, channel or string.
// ok is false for any other kind, including untyped nil and pointers.
func Length(object any) (n int, ok bool) {
	if object == nil {
		return 0, false
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan, reflect.String:
		return rv.Len(), true
	}
	return 0, false
}

// JSONEqual reports whether two values from NormalizeJSON are the same JSON.
func JSONEqual(expected, actual any) bool {
	return reflect.DeepEqual(expected, actual)
}
