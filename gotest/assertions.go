package gotest

import (
	"cmp"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/gotest/require"
)

// TestingT is the interface satisfied by *testing.T, *testing.B, and *T.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
	FailNow()
}

type regexpPattern interface{ string | *regexp.Regexp }

type numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

// --- Must ---

// Must unwraps a (T, error) or (T, bool) return pair, returning val on
// success. On failure it panics.
//
//	conn := gotest.Must(db.Connect(ctx))
func Must[T any](val T, ok any) T { return require.Must(val, ok) }

// --- Equality ---

// Equal checks that expected and actual are deeply equal.
// The type parameter ensures both values share the same type at compile time.
// Stops the test on failure.
func Equal[T any](t TestingT, expected, actual T, msgAndArgs ...any) {
	t.Helper()
	require.Equal(t, expected, actual, msgAndArgs...)
}

// NotEqual checks that expected and actual are not deeply equal.
// Stops the test on failure.
func NotEqual[T any](t TestingT, expected, actual T, msgAndArgs ...any) {
	t.Helper()
	require.NotEqual(t, expected, actual, msgAndArgs...)
}

// --- Zero / Empty ---

// Zero checks that value is the zero value for its type.
// Stops the test on failure.
func Zero[T comparable](t TestingT, value T, msgAndArgs ...any) {
	t.Helper()
	require.Zero(t, value, msgAndArgs...)
}

// NotZero checks that value is not the zero value for its type.
// Stops the test on failure.
func NotZero[T comparable](t TestingT, value T, msgAndArgs ...any) {
	t.Helper()
	require.NotZero(t, value, msgAndArgs...)
}

// Empty checks that a container has zero length or is nil.
// Stops the test on failure.
func Empty(t TestingT, object any, msgAndArgs ...any) {
	t.Helper()
	require.Empty(t, object, msgAndArgs...)
}

// NotEmpty checks that a container has non-zero length.
// Stops the test on failure.
func NotEmpty(t TestingT, object any, msgAndArgs ...any) {
	t.Helper()
	require.NotEmpty(t, object, msgAndArgs...)
}

// --- Bool ---

// True checks that value is true. Stops the test on failure.
func True(t TestingT, value bool, msgAndArgs ...any) {
	t.Helper()
	require.True(t, value, msgAndArgs...)
}

// False checks that value is false. Stops the test on failure.
func False(t TestingT, value bool, msgAndArgs ...any) {
	t.Helper()
	require.False(t, value, msgAndArgs...)
}

// --- Error ---

// NoError checks that err is nil. Stops the test on failure.
func NoError(t TestingT, err error, msgAndArgs ...any) {
	t.Helper()
	require.NoError(t, err, msgAndArgs...)
}

// Error checks that err is not nil. Stops the test on failure.
func Error(t TestingT, err error, msgAndArgs ...any) {
	t.Helper()
	require.Error(t, err, msgAndArgs...)
}

// ErrorIs checks that at least one error in err's chain matches target.
// Stops the test on failure.
func ErrorIs(t TestingT, err, target error, msgAndArgs ...any) {
	t.Helper()
	require.ErrorIs(t, err, target, msgAndArgs...)
}

// ErrorAs checks that at least one error in err's chain matches the type E
// and returns the matched error. Stops the test on failure.
func ErrorAs[E error](t TestingT, err error, msgAndArgs ...any) E {
	t.Helper()
	return require.ErrorAs[E](t, err, msgAndArgs...)
}

// ErrorContains checks that err is not nil and its message contains the
// specified substring. Stops the test on failure.
func ErrorContains(t TestingT, err error, contains string, msgAndArgs ...any) {
	t.Helper()
	require.ErrorContains(t, err, contains, msgAndArgs...)
}

// --- Collection ---

// Contains checks that s contains the element or substring.
// Stops the test on failure.
func Contains(t TestingT, s, contains any, msgAndArgs ...any) {
	t.Helper()
	require.Contains(t, s, contains, msgAndArgs...)
}

// NotContains checks that s does not contain the element or substring.
// Stops the test on failure.
func NotContains(t TestingT, s, contains any, msgAndArgs ...any) {
	t.Helper()
	require.NotContains(t, s, contains, msgAndArgs...)
}

// Len checks that object has the specified length.
// Stops the test on failure.
func Len(t TestingT, object any, length int, msgAndArgs ...any) {
	t.Helper()
	require.Len(t, object, length, msgAndArgs...)
}

// ElementsMatch checks that listA and listB contain the same elements
// regardless of order. Stops the test on failure.
func ElementsMatch[T comparable](t TestingT, listA, listB []T, msgAndArgs ...any) {
	t.Helper()
	require.ElementsMatch(t, listA, listB, msgAndArgs...)
}

// Subset checks that every element in subset is also in list.
// Stops the test on failure.
func Subset[T comparable](t TestingT, list, subset []T, msgAndArgs ...any) {
	t.Helper()
	require.Subset(t, list, subset, msgAndArgs...)
}

// --- Comparison ---

// Greater checks that a > b. Stops the test on failure.
func Greater[T cmp.Ordered](t TestingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	require.Greater(t, a, b, msgAndArgs...)
}

// GreaterOrEqual checks that a >= b. Stops the test on failure.
func GreaterOrEqual[T cmp.Ordered](t TestingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	require.GreaterOrEqual(t, a, b, msgAndArgs...)
}

// Less checks that a < b. Stops the test on failure.
func Less[T cmp.Ordered](t TestingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	require.Less(t, a, b, msgAndArgs...)
}

// LessOrEqual checks that a <= b. Stops the test on failure.
func LessOrEqual[T cmp.Ordered](t TestingT, a, b T, msgAndArgs ...any) {
	t.Helper()
	require.LessOrEqual(t, a, b, msgAndArgs...)
}

// --- String / Regex ---

// Regexp checks that str matches the regular expression rx.
// Stops the test on failure.
func Regexp[P regexpPattern](t TestingT, rx P, str string, msgAndArgs ...any) {
	t.Helper()
	require.Regexp(t, rx, str, msgAndArgs...)
}

// --- Numeric ---

// InDelta checks that expected and actual are within delta of each other.
// Stops the test on failure.
func InDelta[T numeric](t TestingT, expected, actual T, delta float64, msgAndArgs ...any) {
	t.Helper()
	require.InDelta(t, expected, actual, delta, msgAndArgs...)
}

// --- Serialization ---

// JSONEq checks that two JSON values are equivalent, ignoring key order.
// Stops the test on failure.
func JSONEq(t TestingT, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	require.JSONEq(t, expected, actual, msgAndArgs...)
}

// --- Time ---

// TimeWithin checks that expected and actual are within tolerance of each other.
// Stops the test on failure.
func TimeWithin(t TestingT, expected, actual time.Time, tolerance time.Duration, msgAndArgs ...any) {
	t.Helper()
	require.TimeWithin(t, expected, actual, tolerance, msgAndArgs...)
}

// TimeIsNow checks that ts is within tolerance of time.Now().
// Stops the test on failure.
func TimeIsNow(t TestingT, ts time.Time, tolerance time.Duration, msgAndArgs ...any) {
	t.Helper()
	require.TimeIsNow(t, ts, tolerance, msgAndArgs...)
}

// --- Panic ---

// Panics checks that f panics and returns the recovered value.
// Stops the test on failure.
func Panics(t TestingT, f func(), msgAndArgs ...any) any {
	t.Helper()
	return require.Panics(t, f, msgAndArgs...)
}

// --- Async ---

// Eventually checks that condition returns true within waitFor time,
// polling every tick. Stops the test on failure.
func Eventually(t TestingT, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, condition, waitFor, tick, msgAndArgs...)
}
