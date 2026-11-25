package gotest

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type testingT interface {
	Errorf(format string, args ...any)
	FailNow()
}

func fail(t testingT, msg string, msgAndArgs []any) {
	if userMsg := assert.FormatMessage(msgAndArgs); userMsg != "" {
		msg = msg + "\n  message: " + userMsg
	}
	if frame := assert.CallerFrame(); frame != "" {
		msg = frame + ": " + msg
	}
	if tb, ok := t.(testing.TB); ok {
		report(tb, msg) // a *testing.T, B or F passed directly
	} else {
		t.Errorf("%s", msg)
	}
	if st, ok := t.(*T); ok && st.soft {
		return
	}
	t.FailNow()
}

func failf(t testingT, format string, args ...any) {
	fail(t, fmt.Sprintf(format, args...), nil)
}

// --- private helpers ---

type numeric = assert.Numeric

type regexpPattern interface {
	~string | *regexp.Regexp
}

// --- public assertions (alphabetical) ---

// Consistently asserts that fn passes on every poll for the entire waitFor duration.
func Consistently(t testingT, waitFor, tick time.Duration, fn func(poll *R)) {
	if !pollArgs(t, "Consistently", waitFor, tick) {
		return
	}
	tick = min(tick, waitFor)
	deadline := time.Now().Add(waitFor)
	polls := 0
	for {
		polls++
		rec, ok := pollUntil(nameOf(t), fn, pollLimit(deadline, waitFor))
		if !ok {
			failf(t, "Consistently failed on poll %d:\n    %s", polls, abandonedPoll(waitFor))
			return
		}
		if rec.Failed() {
			failf(t, "Consistently failed on poll %d:\n    %s", polls, rec.Message())
			return
		}
		if !time.Now().Before(deadline) {
			return // this poll reached the deadline
		}
		sleepUntil(earliest(time.Now().Add(tick), deadline))
	}
}

// pollArgs fails t when waitFor or tick could not pace a poll, and reports
// whether polling may start.
func pollArgs(t testingT, name string, waitFor, tick time.Duration) bool {
	if waitFor <= 0 || tick <= 0 {
		failf(t, "%s: waitFor and tick must be positive, got %v and %v", name, waitFor, tick)
		return false
	}
	return true
}

// pollLimit is how long a poll may run: until grace past the deadline, or
// past its own start when it began after the deadline. Polls get waitFor as
// grace, max(tick, waitFor) with tick clamped to waitFor, so a poll slower
// than tick is still judged by its result; one running past that has hung.
func pollLimit(deadline time.Time, grace time.Duration) time.Time {
	if now := time.Now(); now.After(deadline) {
		return now.Add(grace)
	}
	return deadline.Add(grace)
}

// abandonedPoll says why a poll that ran past its limit failed.
func abandonedPoll(grace time.Duration) string {
	return fmt.Sprintf("poll still running past its deadline, abandoned %v after it", grace)
}

// The states of a poll: pollUntil and the poll's goroutine each try to move
// it out of pollRunning, and only one of them wins.
const (
	pollRunning int32 = iota
	pollReturned
	pollAbandoned
)

// pollUntil runs fn on a goroutine of its own, as Record does, and gives it
// until limit; ok is false when fn is still running then. A poll given up on
// keeps running, since Go cannot stop it; a panic it raises later is written
// to stderr with its stack, since the test has moved on. The recorder
// carries name, the polling test's, which a snapshot through it is keyed by.
func pollUntil(name string, fn func(*R), limit time.Time) (rec *R, ok bool) {
	r := &R{name: name}
	var state atomic.Int32
	done := make(chan *capturedPanic, 1) // buffered: a poll given up on still exits
	go func() {
		var captured *capturedPanic
		defer func() {
			if state.CompareAndSwap(pollRunning, pollReturned) {
				done <- captured
			} else if captured != nil {
				fmt.Fprintf(os.Stderr, "gotest: a poll abandoned past its deadline panicked: %v\n%s\n", captured.Unwrap(), captured.Stack())
			}
		}()
		defer func() { captured = capturedFrom(recover(), "poll function") }()
		fn(r)
	}()
	timer := time.NewTimer(time.Until(limit))
	defer timer.Stop()
	var captured *capturedPanic
	select {
	case captured = <-done:
	case <-timer.C:
		if state.CompareAndSwap(pollRunning, pollAbandoned) {
			return nil, false
		}
		captured = <-done // it returned as the limit passed
	}
	if captured != nil {
		panic(captured)
	}
	return r, true
}

// earliest is the earlier of two instants.
func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// sleepUntil waits for the clock to reach at, if it has not.
func sleepUntil(at time.Time) {
	if d := time.Until(at); d > 0 {
		time.Sleep(d)
	}
}

// Contains asserts that haystack contains needle (substring for strings, element for slices/arrays, key for maps).
func Contains(t testingT, haystack, needle any, msgAndArgs ...any) {
	found, guard := assert.IncludesElement(haystack, needle)
	if guard != "" {
		fail(t, "Contains failed:\n  "+guard, msgAndArgs)
		return
	}
	if !found {
		fail(t, fmt.Sprintf("Contains failed:\n  %#v does not contain %#v", haystack, needle), msgAndArgs)
	}
}

// ElementsMatch asserts that a and b contain the same elements regardless of order.
func ElementsMatch[V comparable](t testingT, a, b []V, msgAndArgs ...any) {
	if msg := assert.CheckElementsMatch(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// Empty asserts that object is empty: nil, a nil pointer, or a zero-length slice,
// map, array, channel or string. A non-nil pointer is followed to what it points
// to, which must be one of those. Any other type fails with a type guard error,
// a pointer to a struct or a number included (e.g. int, bool, func, *T); use Nil
// or Zero instead.
func Empty(t testingT, object any, msgAndArgs ...any) {
	if !assert.IsEmptyable(object) {
		guard := "Empty: type %T cannot be empty; for nil checks, use Nil; for zero-value checks, use Zero"
		if reflect.ValueOf(object).Kind() == reflect.Ptr {
			guard = "Empty: %T points to a value that cannot be empty; for nil checks, use Nil; for zero-value checks, use Zero"
		}
		fail(t, fmt.Sprintf(guard, object), msgAndArgs)
		return
	}
	if !assert.IsEmpty(object) {
		fail(t, fmt.Sprintf("Empty failed:\n  object is not empty: %#v", object), msgAndArgs)
	}
}

// Equal asserts that expected and actual are deeply equal.
func Equal[V any](t testingT, expected, actual V, msgAndArgs ...any) {
	if msg := assert.CheckEqual(expected, actual); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// Error asserts that err is not nil.
func Error(t testingT, err error, msgAndArgs ...any) {
	if err == nil {
		fail(t, "Error failed:\n  expected an error but got nil", msgAndArgs)
	}
}

// ErrorAs asserts that the error chain of err contains a value of type E and returns it.
func ErrorAs[E error](t testingT, err error, msgAndArgs ...any) E {
	var target E
	if !errors.As(err, &target) {
		fail(t, fmt.Sprintf("ErrorAs failed:\n  error %v: nothing in its chain is of type %s", err, reflect.TypeFor[E]()), msgAndArgs)
	}
	return target
}

// ErrorContains asserts that err is not nil and its message contains the given substring.
// Fails with a type guard error when substr is empty; for non-nil error checks use Error.
func ErrorContains(t testingT, err error, substr string, msgAndArgs ...any) {
	if substr == "" {
		fail(t, "ErrorContains: contains is empty; for non-nil error checks, use Error", msgAndArgs)
		return
	}
	if err == nil {
		fail(t, "ErrorContains failed:\n  expected an error but got nil", msgAndArgs)
		return
	}
	if text := assert.ErrorText(err); !strings.Contains(text, substr) {
		fail(t, fmt.Sprintf("ErrorContains failed:\n  error: %q\n  does not contain: %q", text, substr), msgAndArgs)
	}
}

// ErrorIs asserts that errors.Is(err, target) is true.
// Fails with a type guard error when target is nil; for nil error checks use NoError.
func ErrorIs(t testingT, err, target error, msgAndArgs ...any) {
	if target == nil {
		fail(t, "ErrorIs: target is nil; for nil error checks, use NoError", msgAndArgs)
		return
	}
	if !errors.Is(err, target) {
		fail(t, fmt.Sprintf("ErrorIs failed:\n  error: %v\n  target: %v", err, target), msgAndArgs)
	}
}

// Eventually asserts that fn passes without assertion failures within waitFor, polling every tick.
func Eventually(t testingT, waitFor, tick time.Duration, fn func(poll *R)) {
	if !pollArgs(t, "Eventually", waitFor, tick) {
		return
	}
	tick = min(tick, waitFor)
	deadline := time.Now().Add(waitFor)
	polls := 0
	for {
		polls++
		rec, ok := pollUntil(nameOf(t), fn, pollLimit(deadline, waitFor))
		if !ok {
			failf(t, "Eventually failed after %v (%d polls):\n  %s", waitFor, polls, abandonedPoll(waitFor))
			return
		}
		if !rec.Failed() {
			return
		}
		if !time.Now().Before(deadline) { // this poll reached the deadline
			failf(t, "Eventually failed after %v (%d polls):\n  last failure:\n    %s", waitFor, polls, rec.Message())
			return
		}
		sleepUntil(earliest(time.Now().Add(tick), deadline))
	}
}

// Fail immediately fails the test.
func Fail(t testingT, msgAndArgs ...any) {
	fail(t, "Fail", msgAndArgs)
}

// False asserts that value is false.
func False(t testingT, value bool, msgAndArgs ...any) {
	if value {
		fail(t, "False failed:\n  expected: false\n  actual:   true", msgAndArgs)
	}
}

// Greater asserts that a > b.
func Greater[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	if msg := assert.CheckGreater(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// GreaterOrEqual asserts that a >= b.
func GreaterOrEqual[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	if msg := assert.CheckGreaterOrEqual(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// InDelta asserts that expected and actual differ by at most delta.
func InDelta[V numeric](t testingT, expected, actual V, delta float64, msgAndArgs ...any) {
	if msg := assert.CheckInDelta(expected, actual, delta); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// JSONEq asserts that expected and actual represent the same JSON structure, ignoring key order.
// A string, []byte, json.RawMessage or io.Reader is read as JSON text, as is a
// value of a named string type that implements neither json.Marshaler nor
// encoding.TextMarshaler; text with invalid UTF-8 fails, naming the byte. Any
// other value is marshaled first.
func JSONEq(t testingT, expected, actual any, msgAndArgs ...any) {
	expNorm, err := assert.NormalizeJSON(expected)
	if err != nil {
		fail(t, fmt.Sprintf("JSONEq failed:\n  could not normalize expected: %v", err), msgAndArgs)
		return
	}
	actNorm, err := assert.NormalizeJSON(actual)
	if err != nil {
		fail(t, fmt.Sprintf("JSONEq failed:\n  could not normalize actual: %v", err), msgAndArgs)
		return
	}
	if !assert.JSONEqual(expNorm, actNorm) {
		expJSON, _ := json.Marshal(expNorm)
		actJSON, _ := json.Marshal(actNorm)
		fail(t, fmt.Sprintf("JSONEq failed:\n  expected: %s\n  actual:   %s", expJSON, actJSON), msgAndArgs)
	}
}

// Len asserts that object has the given length. Applies to slices, arrays, maps, channels, and strings.
func Len(t testingT, object any, length int, msgAndArgs ...any) {
	if object == nil {
		fail(t, fmt.Sprintf("Len failed:\n  object is nil, expected length %d", length), msgAndArgs)
		return
	}
	n, ok := assert.Length(object)
	if !ok {
		fail(t, fmt.Sprintf("Len failed:\n  object of type %T does not have a length", object), msgAndArgs)
		return
	}
	if n != length {
		fail(t, fmt.Sprintf("Len failed:\n  expected length %d\n  actual length %d", length, n), msgAndArgs)
	}
}

// Less asserts that a < b.
func Less[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	if msg := assert.CheckLess(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// LessOrEqual asserts that a <= b.
func LessOrEqual[V cmp.Ordered](t testingT, a, b V, msgAndArgs ...any) {
	if msg := assert.CheckLessOrEqual(a, b); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// NoError asserts that err is nil.
func NoError(t testingT, err error, msgAndArgs ...any) {
	if err != nil {
		fail(t, "NoError failed:\n  unexpected error: "+assert.ErrorText(err), msgAndArgs)
	}
}

// NotContains asserts that haystack does NOT contain needle.
func NotContains(t testingT, haystack, needle any, msgAndArgs ...any) {
	found, guard := assert.IncludesElement(haystack, needle)
	if guard != "" {
		fail(t, "NotContains failed:\n  "+guard, msgAndArgs)
		return
	}
	if found {
		fail(t, fmt.Sprintf("NotContains failed:\n  %#v should not contain %#v", haystack, needle), msgAndArgs)
	}
}

// Nil asserts that object is nil. Intended for non-comparable nilable types (slices, maps, funcs).
// Fails with a type guard error for non-nilable types; for comparable nilables use Zero.
func Nil(t testingT, object any, msgAndArgs ...any) {
	if !assert.IsNilable(object) {
		fail(t, fmt.Sprintf("Nil: type %T is not nilable; for zero-value checks, use Zero", object), msgAndArgs)
		return
	}
	if !assert.IsNil(object) {
		fail(t, fmt.Sprintf("Nil failed:\n  expected nil, got: %#v", object), msgAndArgs)
	}
}

// NotEmpty asserts that object is NOT empty: a slice, map, array, channel or string
// of non-zero length. A non-nil pointer is followed to what it points to, which
// must be one of those; a nil pointer is empty. Any other type fails with a type
// guard error, a pointer to a struct or a number included (e.g. int, bool, func,
// *T); use NotNil or NotZero instead.
func NotEmpty(t testingT, object any, msgAndArgs ...any) {
	if !assert.IsEmptyable(object) {
		guard := "NotEmpty: type %T cannot be empty; for nil checks, use NotNil; for zero-value checks, use NotZero"
		if reflect.ValueOf(object).Kind() == reflect.Ptr {
			guard = "NotEmpty: %T points to a value that cannot be empty; for nil checks, use NotNil; for zero-value checks, use NotZero"
		}
		fail(t, fmt.Sprintf(guard, object), msgAndArgs)
		return
	}
	if assert.IsEmpty(object) {
		fail(t, fmt.Sprintf("NotEmpty failed:\n  object is empty: %#v", object), msgAndArgs)
	}
}

// NotNil asserts that object is not nil. Intended for non-comparable nilable types (slices, maps, funcs).
// Fails with a type guard error for non-nilable types; for comparable nilables use NotZero.
func NotNil(t testingT, object any, msgAndArgs ...any) {
	if !assert.IsNilable(object) {
		fail(t, fmt.Sprintf("NotNil: type %T is not nilable; for zero-value checks, use NotZero", object), msgAndArgs)
		return
	}
	if assert.IsNil(object) {
		fail(t, "NotNil failed:\n  expected a non-nil value", msgAndArgs)
	}
}

// NotEqual asserts that expected and actual are NOT deeply equal.
func NotEqual[V any](t testingT, expected, actual V, msgAndArgs ...any) {
	if msg := assert.CheckNotEqual(expected, actual); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// Same asserts that expected and actual are the same pointer. Equal on
// pointers compares what they point at; Same compares where they point.
func Same[V any](t testingT, expected, actual *V, msgAndArgs ...any) {
	if expected != actual {
		fail(t, fmt.Sprintf("Same failed:\n  expected: %p\n  actual:   %p", expected, actual), msgAndArgs)
	}
}

// NotSame asserts that expected and actual are distinct pointers.
func NotSame[V any](t testingT, expected, actual *V, msgAndArgs ...any) {
	if expected == actual {
		fail(t, fmt.Sprintf("NotSame failed:\n  both: %p", actual), msgAndArgs)
	}
}

// NotZero asserts that value is NOT the zero value for its type.
func NotZero[V comparable](t testingT, value V, msgAndArgs ...any) {
	var zero V
	if value == zero {
		fail(t, fmt.Sprintf("NotZero failed:\n  value is the zero value: %#v", value), msgAndArgs)
	}
}

// Panics asserts that f panics and returns the recovered value.
func Panics(t testingT, f func(), msgAndArgs ...any) any {
	recovered, didPanic := assert.DidPanic(f)
	if !didPanic {
		fail(t, "Panics failed:\n  expected the function to panic but it did not", msgAndArgs)
	}
	return recovered
}

// Regexp asserts that str matches the regular expression pattern rx (string or *regexp.Regexp).
func Regexp[P regexpPattern](t testingT, rx P, str string, msgAndArgs ...any) {
	var re *regexp.Regexp
	switch v := any(rx).(type) {
	case *regexp.Regexp:
		if v == nil {
			fail(t, "Regexp failed:\n  regexp is nil", msgAndArgs)
			return
		}
		re = v
	default:
		// Any string kind, named ones included.
		pattern := reflect.ValueOf(v).String()
		var err error
		re, err = compilePattern(pattern)
		if err != nil {
			fail(t, fmt.Sprintf("Regexp failed:\n  invalid pattern %q: %v", pattern, err), msgAndArgs)
			return
		}
	}
	if !re.MatchString(str) {
		fail(t, fmt.Sprintf("Regexp failed:\n  %q does not match pattern %q", str, re.String()), msgAndArgs)
	}
}

// compiledPatterns keeps the patterns Regexp compiled, up to
// maxCompiledPatterns: a table of a few patterns asserted thousands of
// times compiles each once, and a run of ever new patterns stays bounded.
var compiledPatterns = struct {
	sync.RWMutex
	m map[string]*regexp.Regexp
}{m: map[string]*regexp.Regexp{}}

const maxCompiledPatterns = 1024

func compilePattern(pattern string) (*regexp.Regexp, error) {
	compiledPatterns.RLock()
	re, ok := compiledPatterns.m[pattern]
	compiledPatterns.RUnlock()
	if ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	compiledPatterns.Lock()
	defer compiledPatterns.Unlock()
	if len(compiledPatterns.m) >= maxCompiledPatterns {
		for old := range compiledPatterns.m { // map order: an arbitrary entry
			delete(compiledPatterns.m, old)
			break
		}
	}
	compiledPatterns.m[pattern] = re
	return re, nil
}

// Subset asserts that every element of subset exists in list.
func Subset[V comparable](t testingT, list, subset []V, msgAndArgs ...any) {
	if msg := assert.CheckSubset(list, subset); msg != "" {
		fail(t, msg, msgAndArgs)
	}
}

// TimeIsNow asserts that ts is within tolerance of time.Now().
func TimeIsNow(t testingT, ts time.Time, tolerance time.Duration, msgAndArgs ...any) {
	TimeWithin(t, time.Now(), ts, tolerance, msgAndArgs...)
}

// TimeWithin asserts that expected and actual are within tolerance of each other.
func TimeWithin(t testingT, expected, actual time.Time, tolerance time.Duration, msgAndArgs ...any) {
	diff := expected.Sub(actual)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		fail(t, fmt.Sprintf("TimeWithin failed:\n  |%v - %v| = %v exceeds tolerance %v", expected, actual, diff, tolerance), msgAndArgs)
	}
}

// True asserts that value is true.
func True(t testingT, value bool, msgAndArgs ...any) {
	if !value {
		fail(t, "True failed:\n  expected: true\n  actual:   false", msgAndArgs)
	}
}

// Zero asserts that value is the zero value for its type.
func Zero[V comparable](t testingT, value V, msgAndArgs ...any) {
	var zero V
	if value != zero {
		fail(t, fmt.Sprintf("Zero failed:\n  expected: %#v (zero value)\n  actual:   %#v", zero, value), msgAndArgs)
	}
}
