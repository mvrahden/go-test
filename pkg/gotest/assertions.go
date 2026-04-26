package gotest

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type testingT interface {
	Errorf(format string, args ...any)
	FailNow()
}

func fail(t testingT, msg string, msgAndArgs []any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if userMsg := assert.FormatMessage(msgAndArgs); userMsg != "" {
		msg = msg + "\n  message: " + userMsg
	}
	if trace := assert.CallerTrace(); trace != "" {
		msg += trace
	}
	t.Errorf(msg)
	t.FailNow()
}

// Equal asserts that expected and actual are deeply equal.
func Equal[V any](t testingT, expected, actual V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if msg := assert.CheckEqual(expected, actual); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// NotEqual asserts that expected and actual are NOT deeply equal.
func NotEqual[V any](t testingT, expected, actual V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if msg := assert.CheckNotEqual(expected, actual); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// True asserts that value is true.
func True(t testingT, value bool, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if !value {
		fail(t, "True failed:\n  expected: true\n  actual:   false", msgAndArgs)
	}
}

// False asserts that value is false.
func False(t testingT, value bool, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if value {
		fail(t, "False failed:\n  expected: false\n  actual:   true", msgAndArgs)
	}
}

// Zero asserts that value is the zero value for its type.
func Zero[V comparable](t testingT, value V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	var zero V
	if value != zero {
		fail(t, fmt.Sprintf("Zero failed:\n  expected: %#v (zero value)\n  actual:   %#v", zero, value), msgAndArgs)
	}
}

// NotZero asserts that value is NOT the zero value for its type.
func NotZero[V comparable](t testingT, value V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	var zero V
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
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if !isEmpty(object) {
		fail(t, fmt.Sprintf("Empty failed:\n  object is not empty: %#v", object), msgAndArgs)
	}
}

// NotEmpty asserts that object is NOT empty.
func NotEmpty(t testingT, object any, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if isEmpty(object) {
		fail(t, fmt.Sprintf("NotEmpty failed:\n  object is empty: %#v", object), msgAndArgs)
	}
}

// NoError asserts that err is nil.
func NoError(t testingT, err error, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if err != nil {
		fail(t, fmt.Sprintf("NoError failed:\n  unexpected error: %s", err.Error()), msgAndArgs)
	}
}

// Error asserts that err is not nil.
func Error(t testingT, err error, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if err == nil {
		fail(t, "Error failed:\n  expected an error but got nil", msgAndArgs)
	}
}

// ErrorIs asserts that errors.Is(err, target) is true.
func ErrorIs(t testingT, err, target error, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if !errors.Is(err, target) {
		fail(t, fmt.Sprintf("ErrorIs failed:\n  error: %v\n  target: %v", err, target), msgAndArgs)
	}
}

// ErrorAs asserts that errors.As(err, &target) is true and returns the matched error.
func ErrorAs[E error](t testingT, err error, msgAndArgs ...any) E {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	var target E
	if !errors.As(err, &target) {
		fail(t, fmt.Sprintf("ErrorAs failed:\n  error %v does not match expected type", err), msgAndArgs)
	}
	return target
}

// ErrorContains asserts that err is not nil and err.Error() contains the given substring.
func ErrorContains(t testingT, err error, contains string, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if err == nil {
		fail(t, "ErrorContains failed:\n  expected an error but got nil", msgAndArgs)
		return
	}
	if !strings.Contains(err.Error(), contains) {
		fail(t, fmt.Sprintf("ErrorContains failed:\n  error: %q\n  does not contain: %q", err.Error(), contains), msgAndArgs)
	}
}

// Contains asserts that s contains the element `contains`.
// For strings, checks substring. For slices/arrays, checks element presence. For maps, checks key presence.
func Contains(t testingT, s, contains any, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if !includesElement(s, contains) {
		fail(t, fmt.Sprintf("Contains failed:\n  %#v does not contain %#v", s, contains), msgAndArgs)
	}
}

// NotContains asserts that s does NOT contain the element `contains`.
func NotContains(t testingT, s, contains any, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if includesElement(s, contains) {
		fail(t, fmt.Sprintf("NotContains failed:\n  %#v should not contain %#v", s, contains), msgAndArgs)
	}
}

// includesElement checks whether s contains the given element.
func includesElement(s, element any) bool {
	if s == nil {
		return false
	}
	// string substring check
	if str, ok := s.(string); ok {
		sub, ok := element.(string)
		if !ok {
			return false
		}
		return strings.Contains(str, sub)
	}
	rv := reflect.ValueOf(s)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), element) {
				return true
			}
		}
		return false
	case reflect.Map:
		mapKey := reflect.ValueOf(element)
		if !mapKey.IsValid() {
			return false
		}
		return rv.MapIndex(mapKey).IsValid()
	}
	return false
}

// Len asserts that object has the given length.
func Len(t testingT, object any, length int, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if object == nil {
		fail(t, fmt.Sprintf("Len failed:\n  object is nil, expected length %d", length), msgAndArgs)
		return
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan, reflect.String:
		if rv.Len() != length {
			fail(t, fmt.Sprintf("Len failed:\n  expected length %d\n  actual length %d", length, rv.Len()), msgAndArgs)
		}
	default:
		fail(t, fmt.Sprintf("Len failed:\n  object of type %T does not have a length", object), msgAndArgs)
	}
}

// ElementsMatch asserts that listA and listB contain the same elements regardless of order.
func ElementsMatch[V comparable](t testingT, listA, listB []V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if len(listA) != len(listB) {
		fail(t, fmt.Sprintf("ElementsMatch failed:\n  lists have different lengths: %d vs %d", len(listA), len(listB)), msgAndArgs)
		return
	}
	counts := make(map[V]int, len(listA))
	for _, v := range listA {
		counts[v]++
	}
	for _, v := range listB {
		counts[v]--
		if counts[v] < 0 {
			fail(t, fmt.Sprintf("ElementsMatch failed:\n  element %#v is in listB but not enough times in listA", v), msgAndArgs)
			return
		}
	}
}

// Subset asserts that every element of subset exists in list.
func Subset[V comparable](t testingT, list, subset []V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	set := make(map[V]struct{}, len(list))
	for _, v := range list {
		set[v] = struct{}{}
	}
	for _, v := range subset {
		if _, ok := set[v]; !ok {
			fail(t, fmt.Sprintf("Subset failed:\n  element %#v from subset not found in list", v), msgAndArgs)
			return
		}
	}
}

// Greater asserts that a > b.
func Greater[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if msg := assert.CheckGreater(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// GreaterOrEqual asserts that a >= b.
func GreaterOrEqual[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if msg := assert.CheckGreaterOrEqual(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// Less asserts that a < b.
func Less[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if msg := assert.CheckLess(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// LessOrEqual asserts that a <= b.
func LessOrEqual[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	if msg := assert.CheckLessOrEqual(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// regexpPattern constrains Regexp to accept string or *regexp.Regexp.
type regexpPattern interface {
	~string | *regexp.Regexp
}

// Regexp asserts that str matches the regular expression rx.
func Regexp[P regexpPattern](t testingT, rx P, str string, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	var re *regexp.Regexp
	switch v := any(rx).(type) {
	case string:
		var err error
		re, err = regexp.Compile(v)
		if err != nil {
			fail(t, fmt.Sprintf("Regexp failed:\n  invalid pattern %q: %v", v, err), msgAndArgs)
			return
		}
	case *regexp.Regexp:
		re = v
	}
	if !re.MatchString(str) {
		fail(t, fmt.Sprintf("Regexp failed:\n  %q does not match pattern %q", str, re.String()), msgAndArgs)
	}
}

// numeric constrains InDelta to numeric types.
type numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// InDelta asserts that expected and actual are within delta of each other.
func InDelta[V numeric](t testingT, expected, actual V, delta float64, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	diff := math.Abs(float64(expected) - float64(actual))
	if diff > delta {
		fail(t, fmt.Sprintf("InDelta failed:\n  |%v - %v| = %v exceeds delta %v", expected, actual, diff, delta), msgAndArgs)
	}
}

// normalizeJSON converts a value to a JSON-comparable form (map/slice/etc.).
// Accepts string, []byte, or any JSON-marshalable value.
func normalizeJSON(v any) (any, error) {
	var raw []byte
	switch val := v.(type) {
	case string:
		raw = []byte(val)
	case []byte:
		raw = val
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

// JSONEq asserts that expected and actual represent the same JSON structure.
// Both values may be string, []byte, or any JSON-marshalable value.
func JSONEq(t testingT, expected, actual any, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	expNorm, err := normalizeJSON(expected)
	if err != nil {
		fail(t, fmt.Sprintf("JSONEq failed:\n  could not normalize expected: %v", err), msgAndArgs)
		return
	}
	actNorm, err := normalizeJSON(actual)
	if err != nil {
		fail(t, fmt.Sprintf("JSONEq failed:\n  could not normalize actual: %v", err), msgAndArgs)
		return
	}
	if !reflect.DeepEqual(expNorm, actNorm) {
		expJSON, _ := json.Marshal(expNorm)
		actJSON, _ := json.Marshal(actNorm)
		fail(t, fmt.Sprintf("JSONEq failed:\n  expected: %s\n  actual:   %s", expJSON, actJSON), msgAndArgs)
	}
}

// TimeWithin asserts that expected and actual are within the given tolerance of each other.
func TimeWithin(t testingT, expected, actual time.Time, tolerance time.Duration, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	diff := expected.Sub(actual)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		fail(t, fmt.Sprintf("TimeWithin failed:\n  |%v - %v| = %v exceeds tolerance %v", expected, actual, diff, tolerance), msgAndArgs)
	}
}

// TimeIsNow asserts that ts is within tolerance of time.Now().
func TimeIsNow(t testingT, ts time.Time, tolerance time.Duration, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	TimeWithin(t, time.Now(), ts, tolerance, msgAndArgs...)
}

// Panics asserts that f panics when called. It returns the recovered value.
func Panics(t testingT, f func(), msgAndArgs ...any) any {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	recovered, didPanic := didPanic(f)
	if !didPanic {
		fail(t, "Panics failed:\n  expected the function to panic but it did not", msgAndArgs)
	}
	return recovered
}

// didPanic runs f and reports whether it panicked, along with the recovered value.
func didPanic(f func()) (recovered any, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			recovered = r
			panicked = true
		}
	}()
	f()
	return nil, false
}

// Eventually asserts that condition returns true within waitFor, polling every tick.
func Eventually(t testingT, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	timer := time.NewTimer(waitFor)
	defer timer.Stop()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-timer.C:
			fail(t, fmt.Sprintf("Eventually failed:\n  condition did not become true within %v", waitFor), msgAndArgs)
			return
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// Consistently asserts that condition returns true for the entire waitFor duration, polling every tick.
func Consistently(t testingT, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...any) {
	// stdlib compat: mark frame as helper when t is *testing.T or similar
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	timer := time.NewTimer(waitFor)
	defer timer.Stop()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-timer.C:
			return
		case <-ticker.C:
			if !condition() {
				fail(t, fmt.Sprintf("Consistently failed:\n  condition returned false before %v elapsed", waitFor), msgAndArgs)
				return
			}
		}
	}
}
