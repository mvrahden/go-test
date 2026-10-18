package assert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
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
// string kind, an element of a slice or array, a key of a map. guard names
// what the pair cannot be when s is none of those or element is a type s
// could not hold; it is empty when the question was answerable.
func IncludesElement(s, element any) (found bool, guard string) {
	if s == nil {
		return false, ""
	}
	rv := reflect.ValueOf(s)
	switch rv.Kind() {
	case reflect.String:
		ev := reflect.ValueOf(element)
		if !ev.IsValid() || ev.Kind() != reflect.String {
			return false, fmt.Sprintf("a %T cannot hold a %T", s, element)
		}
		return strings.Contains(rv.String(), ev.String()), ""
	case reflect.Slice, reflect.Array:
		ev := reflect.ValueOf(element)
		if ev.IsValid() && !ev.Type().AssignableTo(rv.Type().Elem()) && rv.Type().Elem().Kind() != reflect.Interface {
			return false, fmt.Sprintf("a %T cannot hold a %T", s, element)
		}
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), element) {
				return true, ""
			}
		}
		return false, ""
	case reflect.Map:
		keyType := rv.Type().Key()
		mapKey := reflect.ValueOf(element)
		if !mapKey.IsValid() {
			// An untyped nil is the zero key of a nilable key type.
			switch keyType.Kind() {
			case reflect.Interface, reflect.Ptr, reflect.Chan, reflect.UnsafePointer:
				mapKey = reflect.Zero(keyType)
			default:
				return false, fmt.Sprintf("a %T cannot hold a key of type %T", s, element)
			}
		} else if !mapKey.Type().AssignableTo(keyType) {
			return false, fmt.Sprintf("a %T cannot hold a key of type %T", s, element)
		}
		return rv.MapIndex(mapKey).IsValid(), ""
	}
	return false, fmt.Sprintf("object of type %T is not a string, slice, array, or map", s)
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
	// Numbers stay as written: decoding them into float64 would make
	// 9007199254740993 and 9007199254740992 the same JSON.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	// Anything but whitespace after the value, a stray } or ] included, is an error.
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("failed to unmarshal JSON: trailing data after the value")
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
// Numbers are compared as the exact values they spell, so 1 equals 1.0 and
// 1e2 equals 100, while integers beyond float64's exact range stay apart.
func JSONEqual(expected, actual any) bool {
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok || len(a) != len(e) {
			return false
		}
		for k, ev := range e {
			av, ok := a[k]
			if !ok || !JSONEqual(ev, av) {
				return false
			}
		}
		return true
	case []any:
		a, ok := actual.([]any)
		if !ok || len(a) != len(e) {
			return false
		}
		for i := range e {
			if !JSONEqual(e[i], a[i]) {
				return false
			}
		}
		return true
	case json.Number:
		a, ok := actual.(json.Number)
		if !ok {
			return false
		}
		er, eok := new(big.Rat).SetString(string(e))
		ar, aok := new(big.Rat).SetString(string(a))
		if !eok || !aok {
			return e == a
		}
		return er.Cmp(ar) == 0
	}
	return reflect.DeepEqual(expected, actual)
}
