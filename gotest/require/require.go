package require

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"time"

	assert "github.com/mvrahden/go-test/gotest/require/internal/assert"
)

// check stops the test if pass is false.
func check(t testingT, pass bool) {
	t.Helper()
	if !pass {
		t.FailNow()
	}
}

// --- Must ---

// Must unwraps a (T, error) or (T, bool) return pair, returning val on
// success. On failure it panics, which the test runner catches and reports
// as a test failure with a full stack trace.
//
// Must takes no testingT parameter so that Go's multi-return expansion
// works directly: Must(fn()) where fn returns (T, error) or (T, bool).
//
//	conn := require.Must(db.Connect(ctx))
//	val  := require.Must(myMap[key])
func Must[T any](val T, ok any) T {
	switch v := ok.(type) {
	case nil:
		// nil error — success
	case bool:
		if !v {
			panic("require.Must: expected ok to be true")
		}
	case error:
		panic(fmt.Sprintf("require.Must: unexpected error: %v", v))
	default:
		panic(fmt.Sprintf("require.Must: unsupported check type %T", ok))
	}
	return val
}

// --- Equality ---

// Equal checks that expected and actual are deeply equal using
// reflect.DeepEqual. The type parameter ensures both values share the
// same type at compile time.
//
// Use Equal for same-type Go value comparison where the full in-memory
// representation matters (unexported fields, nil vs empty slices, concrete
// types). Use [JSONEq] instead when comparing across types or when only
// the JSON-serializable surface matters.
//
// Stops the test on failure.
//
//	require.Equal(t, 123, 123)
func Equal[T any](t testingT, expected, actual T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Equal(t, expected, actual, msgAndArgs...))
}

// NotEqual checks that expected and actual are not deeply equal.
// Stops the test on failure.
//
//	require.NotEqual(t, obj1, obj2)
func NotEqual[T any](t testingT, expected, actual T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.NotEqual(t, expected, actual, msgAndArgs...))
}

// --- Zero / Empty ---

// Zero checks that value is the zero value for its type. For pointers,
// interfaces, errors, and channels this means nil. Works with any
// comparable type — use [Empty] for slices and maps instead.
// Stops the test on failure.
//
//	require.Zero(t, count)       // numeric zero
//	require.Zero(t, ptr)         // nil pointer
//	require.Zero(t, err)         // nil error
//	require.Zero(t, str)         // empty string
func Zero[T comparable](t testingT, value T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Zero(t, value, msgAndArgs...))
}

// NotZero checks that value is not the zero value for its type. For
// pointers, interfaces, errors, and channels this means non-nil.
// Works with any comparable type — use [NotEmpty] for slices and maps.
// Stops the test on failure.
//
//	require.NotZero(t, count)    // non-zero number
//	require.NotZero(t, ptr)      // non-nil pointer
//	require.NotZero(t, record.ID) // ID was assigned
func NotZero[T comparable](t testingT, value T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.NotZero(t, value, msgAndArgs...))
}

// Empty checks that a container has zero length or is nil. Use for
// strings, slices, maps, and channels — use [Zero] for comparable
// scalar types like pointers, errors, and numbers.
// Stops the test on failure.
//
//	require.Empty(t, body)       // empty string
//	require.Empty(t, items)      // nil or empty slice
//	require.Empty(t, headers)    // nil or empty map
func Empty(t testingT, object any, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Empty(t, object, msgAndArgs...))
}

// NotEmpty checks that a container has non-zero length. Use for strings,
// slices, maps, and channels — use [NotZero] for comparable scalar types
// like pointers, errors, and numbers.
// Stops the test on failure.
//
//	require.NotEmpty(t, body)    // non-empty string
//	require.NotEmpty(t, items)   // has elements
//	require.NotEmpty(t, headers) // has entries
func NotEmpty(t testingT, object any, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.NotEmpty(t, object, msgAndArgs...))
}

// --- Bool ---

// True checks that value is true. Stops the test on failure.
//
//	require.True(t, ok)
func True(t testingT, value bool, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.True(t, value, msgAndArgs...))
}

// False checks that value is false. Stops the test on failure.
//
//	require.False(t, found)
func False(t testingT, value bool, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.False(t, value, msgAndArgs...))
}

// --- Error ---

// NoError checks that err is nil. Stops the test on failure.
//
//	require.NoError(t, err)
func NoError(t testingT, err error, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.NoError(t, err, msgAndArgs...))
}

// Error checks that err is not nil. Stops the test on failure.
//
//	require.Error(t, err)
func Error(t testingT, err error, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Error(t, err, msgAndArgs...))
}

// ErrorIs checks that at least one error in err's chain matches target
// using errors.Is. Stops the test on failure.
func ErrorIs(t testingT, err, target error, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.ErrorIs(t, err, target, msgAndArgs...))
}

// ErrorAs checks that at least one error in err's chain matches the type E
// and returns the matched error. Stops the test on failure.
//
//	notFound := require.ErrorAs[*NotFoundError](t, err)
func ErrorAs[E error](t testingT, err error, msgAndArgs ...any) E {
	t.Helper()
	var target E
	check(t, assert.ErrorAs(t, err, &target, msgAndArgs...))
	return target
}

// ErrorContains checks that err is not nil and its message contains the
// specified substring. Stops the test on failure.
//
//	require.ErrorContains(t, err, "not found")
func ErrorContains(t testingT, err error, contains string, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.ErrorContains(t, err, contains, msgAndArgs...))
}

// --- Collection ---

// Contains checks that s contains the element or substring. Supports
// strings, slices, arrays, and maps (checks keys).
// Stops the test on failure.
//
//	require.Contains(t, "Hello World", "World")
//	require.Contains(t, []string{"Hello", "World"}, "World")
//	require.Contains(t, map[string]string{"Hello": "World"}, "Hello")
func Contains(t testingT, s, contains any, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Contains(t, s, contains, msgAndArgs...))
}

// NotContains checks that s does not contain the element or substring.
// Stops the test on failure.
func NotContains(t testingT, s, contains any, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.NotContains(t, s, contains, msgAndArgs...))
}

// Len checks that object has the specified length. Works with strings,
// slices, arrays, maps, and channels. Stops the test on failure.
//
//	require.Len(t, items, 3)
func Len(t testingT, object any, length int, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Len(t, object, length, msgAndArgs...))
}

// ElementsMatch checks that listA and listB contain the same elements
// regardless of order. Duplicate count must match.
// Stops the test on failure.
//
//	require.ElementsMatch(t, []int{1, 3, 2, 3}, []int{1, 3, 3, 2})
func ElementsMatch[T comparable](t testingT, listA, listB []T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.ElementsMatch(t, listA, listB, msgAndArgs...))
}

// Subset checks that every element in subset is also in list.
// Stops the test on failure.
//
//	require.Subset(t, []int{1, 2, 3}, []int{1, 2})
func Subset[T comparable](t testingT, list, subset []T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Subset(t, list, subset, msgAndArgs...))
}

// --- Comparison ---

// Greater checks that a > b. Stops the test on failure.
//
//	require.Greater(t, 2, 1)
//	require.Greater(t, 2.0, 1.0)
//	require.Greater(t, "b", "a")
func Greater[T cmp.Ordered](t testingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Greater(t, a, b, msgAndArgs...))
}

// GreaterOrEqual checks that a >= b. Stops the test on failure.
//
//	require.GreaterOrEqual(t, 2, 1)
//	require.GreaterOrEqual(t, 2, 2)
func GreaterOrEqual[T cmp.Ordered](t testingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.GreaterOrEqual(t, a, b, msgAndArgs...))
}

// Less checks that a < b. Stops the test on failure.
//
//	require.Less(t, 1, 2)
//	require.Less(t, 1.0, 2.0)
//	require.Less(t, "a", "b")
func Less[T cmp.Ordered](t testingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Less(t, a, b, msgAndArgs...))
}

// LessOrEqual checks that a <= b. Stops the test on failure.
//
//	require.LessOrEqual(t, 1, 2)
//	require.LessOrEqual(t, 2, 2)
func LessOrEqual[T cmp.Ordered](t testingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.LessOrEqual(t, a, b, msgAndArgs...))
}

// --- String / Regex ---

// Regexp checks that str matches the regular expression rx. The rx
// argument may be a *regexp.Regexp or a string pattern.
// Stops the test on failure.
//
//	require.Regexp(t, regexp.MustCompile("start"), "it's starting")
//	require.Regexp(t, "start...$", "it's not starting")
func Regexp[P regexpPattern](t testingT, rx P, str string, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Regexp(t, rx, str, msgAndArgs...))
}

// --- Numeric ---

// InDelta checks that expected and actual are within delta of each other.
// Stops the test on failure.
//
//	require.InDelta(t, math.Pi, 22/7.0, 0.01)
func InDelta[T numeric](t testingT, expected, actual T, delta float64, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.InDelta(t, expected, actual, delta, msgAndArgs...))
}

// --- Serialization ---

// toJSON converts a value to a JSON string. Raw JSON types (string, []byte,
// json.RawMessage) pass through; io.Reader is read and treated as raw JSON;
// everything else is marshaled. This means http.Request.Body,
// http.Response.Body, and *bytes.Buffer all work directly.
func toJSON(t testingT, v any, label string, msgAndArgs ...any) (string, bool) {
	t.Helper()
	switch val := v.(type) {
	case string:
		return val, true
	case []byte:
		return string(val), true
	case json.RawMessage:
		return string(val), true
	case io.Reader:
		b, err := io.ReadAll(val)
		if err != nil {
			assert.Fail(t, fmt.Sprintf("%s: failed to read: %s", label, err), msgAndArgs...)
			return "", false
		}
		return string(b), true
	default:
		b, err := json.Marshal(v)
		if err != nil {
			assert.Fail(t, fmt.Sprintf("%s is not JSON-serializable: %s", label, err), msgAndArgs...)
			return "", false
		}
		return string(b), true
	}
}

// JSONEq checks that two JSON values are equivalent, ignoring key order.
// Accepts raw JSON (string, []byte, json.RawMessage) or any marshalable
// Go value (structs, maps, slices, etc.). Arguments may be different types.
//
// Use JSONEq when comparing across type boundaries (e.g. an API response
// body against a Go struct), or when only the JSON-visible structure
// matters (exported and tagged fields, key order irrelevant). Use [Equal]
// instead when both values share the same Go type and the full in-memory
// representation matters.
//
// Stops the test on failure.
//
//	require.JSONEq(t, `{"name":"alice"}`, user)
//	require.JSONEq(t, expectedMap, resp.Body)
func JSONEq(t testingT, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	exp, ok := toJSON(t, expected, "Expected", msgAndArgs...)
	if !ok {
		t.FailNow()
		return
	}
	act, ok := toJSON(t, actual, "Actual", msgAndArgs...)
	if !ok {
		t.FailNow()
		return
	}
	check(t, assert.JSONEq(t, exp, act, msgAndArgs...))
}

// --- Time ---

// TimeWithin checks that expected and actual are within tolerance of each
// other. Use for timestamp equality across precisions (e.g. DB round-trips)
// or expiry range checks. Stops the test on failure.
//
//	require.TimeWithin(t, time.Now().Add(time.Hour), token.ExpiresAt, 5*time.Second)
func TimeWithin(t testingT, expected, actual time.Time, tolerance time.Duration, msgAndArgs ...any) {
	t.Helper()
	diff := expected.Sub(actual)
	if diff < 0 {
		diff = -diff
	}
	if diff <= tolerance {
		return
	}
	assert.Fail(t, fmt.Sprintf(
		"times differ by %s (tolerance %s)\n"+
			"\texpected: %s\n"+
			"\tactual:   %s",
		diff, tolerance,
		expected.Format(time.RFC3339Nano),
		actual.Format(time.RFC3339Nano),
	), msgAndArgs...)
	t.FailNow()
}

// TimeIsNow checks that ts is within tolerance of time.Now().
// Use for verifying timestamps that were set during the operation under
// test (CreatedAt, UpdatedAt, etc.). Stops the test on failure.
//
//	require.TimeIsNow(t, record.CreatedAt, time.Second)
func TimeIsNow(t testingT, ts time.Time, tolerance time.Duration, msgAndArgs ...any) {
	t.Helper()
	TimeWithin(t, time.Now(), ts, tolerance, msgAndArgs...)
}

// --- Panic ---

// Panics checks that f panics and returns the recovered value.
// Stops the test on failure.
//
//	v := require.Panics(t, func() { panic("boom") })
func Panics(t testingT, f func(), msgAndArgs ...any) any {
	t.Helper()
	ok, val := assert.Panics(t, f, msgAndArgs...)
	check(t, ok)
	return val
}

// --- Async ---

// Eventually checks that condition returns true within waitFor time,
// polling every tick. Stops the test on failure.
//
//	require.Eventually(t, func() bool { return true }, time.Second, 10*time.Millisecond)
func Eventually(t testingT, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...any) {
	t.Helper()
	check(t, assert.Eventually(t, condition, waitFor, tick, msgAndArgs...))
}
