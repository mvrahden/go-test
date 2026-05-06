package assert_test

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

func newFormatSpy(t *testing.T) (*bytes.Buffer, func(format string, args ...any)) {
	t.Helper()
	buf := bytes.NewBufferString("")
	t.Cleanup(func() {
		buf.Reset()
	})
	return buf, func(format string, args ...any) {
		_, err := fmt.Fprintf(buf, format, args...)
		if err != nil {
			t.Fatalf("failed writing to buffer: %s", err)
		}
	}
}

func assertSuccess(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	t.Cleanup(func() {
		buf.Reset()
	})
	if buf.Len() > 0 {
		t.Fatalf("assertion failed, but was expected to succeed. got: %q", buf)
	}
}

func assertFail(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	t.Cleanup(func() {
		buf.Reset()
	})
	if buf.Len() == 0 {
		t.Fatalf("assertion succeeded, but was expected to fail.")
	}
}

func assertFailMsg(t *testing.T, buf *bytes.Buffer, expected string) {
	t.Helper()
	t.Cleanup(func() {
		buf.Reset()
	})
	if buf.Len() == 0 {
		t.Fatalf("assertion succeeded, but was expected to fail.")
	}
	if actual := buf.String(); actual != expected {
		t.Fatalf("assertion error does not equal expected error.\n\tgot:  %q\n\twant: %q", actual, expected)
	}
}

// ---------------------------------------------------------------------------
// IsTrue
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_IsTrue_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{true, bool(true), any(true)} {
		assert.NewAssertionContextForTest(v, fmtFn).IsTrue()
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_IsTrue_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	// non-bool values and bool false all fail
	for _, v := range []any{false, "abc", 123, nil} {
		assert.NewAssertionContextForTest(v, fmtFn).IsTrue()
		assertFail(t, buf)
	}
}

func Test_BaseAssertionContext_IsTrue_Fail_ExactMessage(t *testing.T) {
	t.Run("bool false", func(t *testing.T) {
		buf, fmtFn := newFormatSpy(t)
		assert.NewAssertionContextForTest(false, fmtFn).IsTrue()
		assertFailMsg(t, buf, "True failed:\n  got: false")
	})
	t.Run("string value", func(t *testing.T) {
		buf, fmtFn := newFormatSpy(t)
		assert.NewAssertionContextForTest("abc", fmtFn).IsTrue()
		assertFailMsg(t, buf, "True failed:\n  got: abc")
	})
}

// ---------------------------------------------------------------------------
// IsFalse
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_IsFalse_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{false, bool(false), any(false)} {
		assert.NewAssertionContextForTest(v, fmtFn).IsFalse()
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_IsFalse_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{true, "abc", 123, nil} {
		assert.NewAssertionContextForTest(v, fmtFn).IsFalse()
		assertFail(t, buf)
	}
}

func Test_BaseAssertionContext_IsFalse_Fail_ExactMessage(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(true, fmtFn).IsFalse()
	assertFailMsg(t, buf, "False failed:\n  got: true")
}

// ---------------------------------------------------------------------------
// Equal (formerly IsEqualTo)
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_Equal_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{true, false, 123, 123.123, "abc", (*string)(nil)} {
		assert.NewAssertionContextForTest(v, fmtFn).Equal(v)
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_Equal_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	testCases := []struct{ v1, v2 any }{
		{true, false},
		{123, 456},
		{"abc", "def"},
		{math.NaN(), math.NaN()},
	}
	for _, tc := range testCases {
		assert.NewAssertionContextForTest(tc.v1, fmtFn).Equal(tc.v2)
		assertFail(t, buf)
	}
}

func Test_BaseAssertionContext_Equal_Fail_MessageFormat(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	// Verify CheckEqual format is used — message starts with "Equal failed:"
	assert.NewAssertionContextForTest("abc", fmtFn).Equal("def")
	msg := buf.String()
	buf.Reset()
	if len(msg) == 0 || msg[:len("Equal failed:")] != "Equal failed:" {
		t.Fatalf("expected message to start with 'Equal failed:', got: %q", msg)
	}
}

// IsEqualTo legacy alias works the same way
func Test_BaseAssertionContext_IsEqualTo_LegacyAlias(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("abc", fmtFn).IsEqualTo("abc")
	assertSuccess(t, buf)

	assert.NewAssertionContextForTest("abc", fmtFn).IsEqualTo("def")
	assertFail(t, buf)
}

// ---------------------------------------------------------------------------
// IsZero
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_IsZero_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{
		false, 0, int64(0), float64(0), "", (*string)(nil),
		[]byte(nil), (map[byte]any)(nil), (chan bool)(nil),
	} {
		assert.NewAssertionContextForTest(v, fmtFn).IsZero()
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_IsZero_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{true, 1, "abc", math.NaN()} {
		assert.NewAssertionContextForTest(v, fmtFn).IsZero()
		assertFail(t, buf)
	}
}

func Test_BaseAssertionContext_IsZero_Fail_NilIsError(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	// untyped nil → error (not a typed zero)
	assert.NewAssertionContextForTest(nil, fmtFn).IsZero()
	assertFailMsg(t, buf, "Zero failed:\n  nil is not a typed zero value")
}

func Test_BaseAssertionContext_IsZero_Fail_ExactMessage(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(true, fmtFn).IsZero()
	assertFailMsg(t, buf, "Zero failed:\n  got: true")
}

// ---------------------------------------------------------------------------
// IsNotZero
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_IsNotZero_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{true, 1, "abc", math.NaN()} {
		assert.NewAssertionContextForTest(v, fmtFn).IsNotZero()
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_IsNotZero_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{false, 0, "", (*string)(nil)} {
		assert.NewAssertionContextForTest(v, fmtFn).IsNotZero()
		assertFail(t, buf)
	}
}

// ---------------------------------------------------------------------------
// IsEmpty
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_IsEmpty_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{
		"",
		[]byte(nil),
		make([]byte, 0, 16),
		[0]byte{},
		(map[byte]any)(nil),
		make(map[byte]any),
		(chan bool)(nil),
		make(chan bool, 16),
	} {
		assert.NewAssertionContextForTest(v, fmtFn).IsEmpty()
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_IsEmpty_Fail_NonEmpty(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{"abc", []byte{1, 2, 3}, map[string]any{"a": 1}} {
		assert.NewAssertionContextForTest(v, fmtFn).IsEmpty()
		assertFail(t, buf)
	}
}

func Test_BaseAssertionContext_IsEmpty_Fail_InvalidType(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{true, 123, struct{ a bool }{true}} {
		assert.NewAssertionContextForTest(v, fmtFn).IsEmpty()
		assertFail(t, buf)
	}
}

func Test_BaseAssertionContext_IsEmpty_Fail_ExactMessage(t *testing.T) {
	t.Run("non-empty string", func(t *testing.T) {
		buf, fmtFn := newFormatSpy(t)
		assert.NewAssertionContextForTest("abc", fmtFn).IsEmpty()
		assertFailMsg(t, buf, "Empty failed:\n  got length: 3")
	})
	t.Run("invalid type", func(t *testing.T) {
		buf, fmtFn := newFormatSpy(t)
		assert.NewAssertionContextForTest(true, fmtFn).IsEmpty()
		assertFailMsg(t, buf, "Empty failed:\n  value of type <bool> cannot be empty")
	})
}

// ---------------------------------------------------------------------------
// HasLength
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_HasLength_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	testCases := []struct {
		v any
		l int
	}{
		{"", 0},
		{"abc", 3},
		{[]byte{1, 2, 3}, 3},
		{[3]byte{1, 2, 3}, 3},
		{map[byte]any{1: 1, 2: 2}, 2},
		{func() chan bool { c := make(chan bool, 5); c <- true; return c }(), 1},
		// pointer dereference
		{func() *string { s := "abc"; return &s }(), 3},
	}
	for _, tc := range testCases {
		assert.NewAssertionContextForTest(tc.v, fmtFn).HasLength(tc.l)
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_HasLength_Fail_WrongLength(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("ab", fmtFn).HasLength(3)
	assertFail(t, buf)

	assert.NewAssertionContextForTest([]byte{1, 2}, fmtFn).HasLength(3)
	assertFail(t, buf)
}

func Test_BaseAssertionContext_HasLength_Fail_InvalidType(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(123, fmtFn).HasLength(3)
	assertFail(t, buf)

	assert.NewAssertionContextForTest(nil, fmtFn).HasLength(0)
	assertFail(t, buf)
}

func Test_BaseAssertionContext_HasLength_Fail_ExactMessage(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("ab", fmtFn).HasLength(3)
	assertFailMsg(t, buf, "HasLength failed:\n  expected length: 3\n  actual length:   2")
}

// ---------------------------------------------------------------------------
// HasCapacity
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_HasCapacity_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	testCases := []struct {
		v any
		c int
	}{
		{make([]byte, 0, 16), 16},
		{[]byte{1, 2, 3}, 3},
		{[3]byte{1, 2, 3}, 3},
		{make(chan bool, 16), 16},
		{(chan bool)(nil), 0},
	}
	for _, tc := range testCases {
		assert.NewAssertionContextForTest(tc.v, fmtFn).HasCapacity(tc.c)
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_HasCapacity_Fail_WrongCapacity(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest([]byte{1, 2}, fmtFn).HasCapacity(3)
	assertFail(t, buf)

	assert.NewAssertionContextForTest(make(chan bool, 5), fmtFn).HasCapacity(3)
	assertFail(t, buf)
}

func Test_BaseAssertionContext_HasCapacity_Fail_InvalidType(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("abc", fmtFn).HasCapacity(3)
	assertFail(t, buf)

	assert.NewAssertionContextForTest(map[string]any{"a": 1}, fmtFn).HasCapacity(1)
	assertFail(t, buf)
}

func Test_BaseAssertionContext_HasCapacity_Fail_ExactMessage(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest([]byte{1, 2}, fmtFn).HasCapacity(3)
	assertFailMsg(t, buf, "HasCapacity failed:\n  expected capacity: 3\n  actual capacity:   2")
}

// ---------------------------------------------------------------------------
// Contains
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_Contains_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("hello world", fmtFn).Contains("world")
	assertSuccess(t, buf)

	assert.NewAssertionContextForTest([]int{1, 2, 3}, fmtFn).Contains(2)
	assertSuccess(t, buf)

	assert.NewAssertionContextForTest([3]string{"a", "b", "c"}, fmtFn).Contains("b")
	assertSuccess(t, buf)
}

func Test_BaseAssertionContext_Contains_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("hello", fmtFn).Contains("xyz")
	assertFail(t, buf)

	assert.NewAssertionContextForTest([]int{1, 2, 3}, fmtFn).Contains(99)
	assertFail(t, buf)
}

// ---------------------------------------------------------------------------
// NoError
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_NoError_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(nil, fmtFn).NoError()
	assertSuccess(t, buf)

	var err error
	assert.NewAssertionContextForTest(err, fmtFn).NoError()
	assertSuccess(t, buf)
}

func Test_BaseAssertionContext_NoError_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(fmt.Errorf("something went wrong"), fmtFn).NoError()
	assertFail(t, buf)
}

func Test_BaseAssertionContext_NoError_Fail_ExactMessage(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(fmt.Errorf("oops"), fmtFn).NoError()
	assertFailMsg(t, buf, "NoError failed:\n  got: oops")
}

func Test_BaseAssertionContext_NoError_Fail_NonErrorType(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("not an error", fmtFn).NoError()
	assertFailMsg(t, buf, "NoError failed:\n  value of type string is not an error")
}

// ---------------------------------------------------------------------------
// NotEqual
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_NotEqual_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	testCases := []struct{ v1, v2 any }{
		{1, 2},
		{"abc", "def"},
		{true, false},
	}
	for _, tc := range testCases {
		assert.NewAssertionContextForTest(tc.v1, fmtFn).NotEqual(tc.v2)
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_NotEqual_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{42, "abc", true} {
		assert.NewAssertionContextForTest(v, fmtFn).NotEqual(v)
		assertFail(t, buf)
	}
}

// ---------------------------------------------------------------------------
// NotContains
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_NotContains_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("hello", fmtFn).NotContains("xyz")
	assertSuccess(t, buf)

	assert.NewAssertionContextForTest([]int{1, 2, 3}, fmtFn).NotContains(99)
	assertSuccess(t, buf)
}

func Test_BaseAssertionContext_NotContains_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("hello world", fmtFn).NotContains("world")
	assertFail(t, buf)

	assert.NewAssertionContextForTest([]int{1, 2, 3}, fmtFn).NotContains(2)
	assertFail(t, buf)
}

func Test_BaseAssertionContext_NotContains_Fail_InvalidType(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(123, fmtFn).NotContains("a")
	assertFail(t, buf)
}

func Test_BaseAssertionContext_NotContains_Fail_StringNonStringElement(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("hello", fmtFn).NotContains(123)
	assertFail(t, buf)
}

// ---------------------------------------------------------------------------
// NotEmpty
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_NotEmpty_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{"abc", []byte{1, 2}, map[string]any{"a": 1}} {
		assert.NewAssertionContextForTest(v, fmtFn).NotEmpty()
		assertSuccess(t, buf)
	}
}

func Test_BaseAssertionContext_NotEmpty_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	for _, v := range []any{nil, "", []byte(nil), make([]byte, 0)} {
		assert.NewAssertionContextForTest(v, fmtFn).NotEmpty()
		assertFail(t, buf)
	}
}

// ---------------------------------------------------------------------------
// IsError
// ---------------------------------------------------------------------------

func Test_BaseAssertionContext_IsError_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(fmt.Errorf("boom"), fmtFn).IsError()
	assertSuccess(t, buf)
}

func Test_BaseAssertionContext_IsError_Fail_Nil(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest(nil, fmtFn).IsError()
	assertFailMsg(t, buf, "IsError failed:\n  expected an error but got nil")
}

func Test_BaseAssertionContext_IsError_Fail_NonError(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)

	assert.NewAssertionContextForTest("not an error", fmtFn).IsError()
	assertFailMsg(t, buf, "IsError failed:\n  value of type string is not an error")
}

// ---------------------------------------------------------------------------
// NewAssertionContext (via *testing.T)
// ---------------------------------------------------------------------------

func Test_NewAssertionContext_Equal_Success(t *testing.T) {
	assert.NewAssertionContext(42, t).Equal(42)
}

func Test_NewAssertionContext_Contains_String(t *testing.T) {
	assert.NewAssertionContext("hello world", t).Contains("world")
}
