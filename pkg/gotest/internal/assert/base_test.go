package assert_test

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

var (
	// 0x1041b9c10
	PtrRegex = regexp.MustCompile(`0x[0-9a-f]{4,16}`)
)

func maskPtr(v string) string {
	return PtrRegex.ReplaceAllString(v, "<POINTER_REF>")
}

func newFormatSpy(t *testing.T) (*bytes.Buffer, func(format string, args ...any)) {
	t.Helper()
	buf := bytes.NewBufferString("")
	t.Cleanup(func() {
		buf.Reset()
	})
	return buf, func(format string, args ...any) {
		_, err := buf.WriteString(fmt.Sprintf(format, args...))
		if err != nil {
			t.Fatalf("failed writing to buffer: %s", err)
		}
	}
}

func Test_BaseAsserter_IsTrue_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v any
	}{
		{v: true},
		{v: bool(true)},
		{v: any(true)},
		// {v: func() *bool { a := true; return &a }()},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v).IsTrue()
			assertSuccess(t, buf)
		})
	}
}

func Test_BaseAsserter_IsTrue_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v1  any
		out string
	}{
		{v1: false, out: "false is not true"},
		{v1: any(false), out: "false is not true"},
		{v1: "abc", out: "abc is not true"},
		{v1: map[string]any{"1": "abc"}, out: "map[1:abc] is not true"},
		{v1: map[string]any{"1": func() {}}, out: "map[1:<POINTER_REF>] is not true"},
		{v1: [1]byte{1}, out: "[1] is not true"},
		{v1: []byte{1, 2}, out: "[1 2] is not true"},
		{v1: func() chan bool { c := make(chan bool, 123); c <- true; return c }(), out: "<POINTER_REF> is not true"},
		{v1: func() *string { c := "abc"; return &c }(), out: "<POINTER_REF> is not true"},
		{v1: func() **string { c := "abc"; d := &c; return &d }(), out: "<POINTER_REF> is not true"},
		// cannot be empty
		{v1: func() **string { return nil }(), out: "<nil> is not true"},
		{v1: math.Inf(1), out: "+Inf is not true"},
		{v1: 123, out: "123 is not true"},
		{v1: 123.123, out: "123.123 is not true"},
		{v1: complex128(123), out: "(123+0i) is not true"},
		{v1: math.NaN(), out: "NaN is not true"},
		{v1: func() {}, out: "<POINTER_REF> is not true"},
		{v1: struct{ a bool }{true}, out: "{true} is not true"},
		{v1: nil, out: "<nil> is not true"},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v1).IsTrue()
			assertFail(t, buf, tc.out)
		})
	}
}

func Test_BaseAsserter_IsFalse_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v any
	}{
		{v: false},
		{v: bool(false)},
		{v: any(false)},
		// {v: func() *bool { a := true; return &a }()},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v).IsFalse()
			assertSuccess(t, buf)
		})
	}
}

func Test_BaseAsserter_IsFalse_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v1  any
		out string
	}{
		{v1: true, out: "true is not false"},
		{v1: any(true), out: "true is not false"},
		{v1: "abc", out: "abc is not false"},
		{v1: map[string]any{"1": "abc"}, out: "map[1:abc] is not false"},
		{v1: map[string]any{"1": func() {}}, out: "map[1:<POINTER_REF>] is not false"},
		{v1: [1]byte{1}, out: "[1] is not false"},
		{v1: []byte{1, 2}, out: "[1 2] is not false"},
		{v1: func() chan bool { c := make(chan bool, 123); c <- true; return c }(), out: "<POINTER_REF> is not false"},
		{v1: func() *string { c := "abc"; return &c }(), out: "<POINTER_REF> is not false"},
		{v1: func() **string { c := "abc"; d := &c; return &d }(), out: "<POINTER_REF> is not false"},
		// cannot be empty
		{v1: func() **string { return nil }(), out: "<nil> is not false"},
		{v1: math.Inf(1), out: "+Inf is not false"},
		{v1: 123, out: "123 is not false"},
		{v1: 123.123, out: "123.123 is not false"},
		{v1: complex128(123), out: "(123+0i) is not false"},
		{v1: math.NaN(), out: "NaN is not false"},
		{v1: func() {}, out: "<POINTER_REF> is not false"},
		{v1: struct{ a bool }{true}, out: "{true} is not false"},
		{v1: nil, out: "<nil> is not false"},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v1).IsFalse()
			assertFail(t, buf, tc.out)
		})
	}
}

func Test_BaseAsserter_IsEqualTo_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []any{
		true,
		false,
		123,
		123.123,
		math.Inf(1),
		math.Inf(-1),
		(*string)(nil),
		"abc",
		rune('🙌'),
		map[string]any{"Hello": 1, "World": 2},
	}
	for idx, v := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(v).IsEqualTo(v)
			assertSuccess(t, buf)
		})
	}
}

func Test_BaseAsserter_IsEqualTo_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v1, v2 any
		out    string
	}{
		{v1: true, v2: false, out: "true is not equal to false"},
		{v1: math.Inf(1), v2: math.Inf(-1), out: "+Inf is not equal to -Inf"},
		{v1: 123, v2: 456, out: "123 is not equal to 456"},
		{v1: 123.123, v2: 456, out: "123.123 is not equal to 456"},
		{v1: complex128(123), v2: complex128(456), out: "(123+0i) is not equal to (456+0i)"},
		{v1: math.NaN(), v2: math.NaN(), out: "NaN is not equal to NaN"},
		{v1: make(chan bool, 123), v2: make(chan bool, 123), out: "Type<chan bool> is not equal to Type<chan bool>"},
		{v1: 123, v2: "abc", out: "123 is not equal to abc"},
		{v1: func() {}, v2: func() {}, out: "Type<func()> is not equal to Type<func()>"},
		{v1: map[string]any{"1": "abc"}, v2: 123, out: "map[1:abc] is not equal to 123"},
		{v1: map[string]any{"1": func() {}}, v2: map[string]any{"1": func() {}}, out: "map[1:<POINTER_REF>] is not equal to map[1:<POINTER_REF>]"},
		{v1: make(chan bool, 123), v2: 123, out: "Type<chan bool> is not equal to 123"},
		{v1: "abc", v2: "def", out: "abc is not equal to def"},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v1).IsEqualTo(tc.v2)
			assertFail(t, buf, tc.out)
		})
	}
}

func Test_BaseAsserter_IsZero_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []any{
		false,
		bool(false),
		0,
		int(0),
		int8(0),
		int16(0),
		int32(0),
		int64(0),
		uint(0),
		uint8(0),
		uint16(0),
		uint32(0),
		uint64(0),
		float32(0),
		float64(0),
		complex64(0),
		complex128(0),
		rune(0),
		byte(0),
		"",
		string(""),
		[]byte(nil),
		[16]byte{},
		(*[]byte)(nil),
		(**[]byte)(nil),
		(***[]byte)(nil),
		(map[byte]any)(nil),
		(chan bool)(nil),
		(func() string)(nil),
		(*string)(nil),
	}
	for idx, v := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(v).IsZero()
			assertSuccess(t, buf)
		})
	}
}

func Test_BaseAsserter_IsZero_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v1  any
		out string
	}{
		// not zero
		{v1: true, out: "true is not zero"},
		{v1: math.Inf(1), out: "+Inf is not zero"},
		{v1: 123, out: "123 is not zero"},
		{v1: 123.123, out: "123.123 is not zero"},
		{v1: complex128(123), out: "(123+0i) is not zero"},
		{v1: math.NaN(), out: "NaN is not zero"},
		{v1: make(chan bool, 123), out: "<POINTER_REF> is not zero"},
		{v1: 123, out: "123 is not zero"},
		{v1: func() {}, out: "<POINTER_REF> is not zero"},
		{v1: map[string]any{"1": "abc"}, out: "map[1:abc] is not zero"},
		{v1: map[string]any{"1": func() {}}, out: "map[1:<POINTER_REF>] is not zero"},
		{v1: make(chan bool, 123), out: "<POINTER_REF> is not zero"},
		{v1: "abc", out: "abc is not zero"},
		{v1: struct{ a bool }{true}, out: "{true} is not zero"},
		// cannot be zero
		{v1: nil, out: "<nil> can not be zero"},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v1).IsZero()
			assertFail(t, buf, tc.out)
		})
	}
}

func Test_BaseAsserter_IsEmpty_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []any{
		"",
		string(""),
		[]byte(nil),
		make([]byte, 0, 16),
		[0]byte{},
		(map[byte]any)(nil),
		make(map[byte]any, 16),
		(chan bool)(nil),
		make(chan bool, 16),
	}
	for idx, v := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(v).IsEmpty()
			assertSuccess(t, buf)
		})
	}
}

func Test_BaseAsserter_IsEmpty_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v1  any
		out string
	}{
		// not empty
		{v1: "abc", out: "is not empty (actual length = 3)"},
		{v1: map[string]any{"1": "abc"}, out: "is not empty (actual length = 1)"},
		{v1: map[string]any{"1": func() {}}, out: "is not empty (actual length = 1)"},
		{v1: [1]byte{1}, out: "is not empty (actual length = 1)"},
		{v1: []byte{1, 2, 3}, out: "is not empty (actual length = 3)"},
		{v1: func() chan bool { c := make(chan bool, 123); c <- true; return c }(), out: "is not empty (actual length = 1)"},
		{v1: func() *string { c := "abc"; return &c }(), out: "is not empty (actual length = 3)"},
		{v1: func() **string { c := "abc"; d := &c; return &d }(), out: "is not empty (actual length = 3)"},
		// cannot be empty
		{v1: func() **string { return nil }(), out: "value of type <ptr> can not be empty"},
		{v1: true, out: "value of type <bool> can not be empty"},
		{v1: math.Inf(1), out: "value of type <float64> can not be empty"},
		{v1: 123, out: "value of type <int> can not be empty"},
		{v1: 123.123, out: "value of type <float64> can not be empty"},
		{v1: complex128(123), out: "value of type <complex128> can not be empty"},
		{v1: math.NaN(), out: "value of type <float64> can not be empty"},
		{v1: 123, out: "value of type <int> can not be empty"},
		{v1: func() {}, out: "value of type <func> can not be empty"},
		{v1: struct{ a bool }{true}, out: "value of type <struct> can not be empty"},
		{v1: nil, out: "value of type <invalid> can not be empty"},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v1).IsEmpty()
			assertFail(t, buf, tc.out)
		})
	}
}

func Test_BaseAsserter_HasLength_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v any
		l int
	}{
		{v: "", l: 0},
		{v: string(""), l: 0},
		{v: []byte(nil), l: 0},
		{v: make([]byte, 0, 16), l: 0},
		{v: [0]byte{}, l: 0},
		{v: (map[byte]any)(nil), l: 0},
		{v: make(map[byte]any, 16), l: 0},
		{v: (chan bool)(nil), l: 0},
		{v: make(chan bool, 16), l: 0},
		{v: "abc", l: 3},
		{v: string("abc"), l: 3},
		{v: []byte{1, 2, 3}, l: 3},
		{v: make([]byte, 3, 16), l: 3},
		{v: [3]byte{1, 2, 3}, l: 3},
		{v: map[byte]any{1: 1, 2: 2, 3: 3}, l: 3},
		{v: func() chan bool { c := make(chan bool, 123); c <- true; c <- false; c <- true; return c }(), l: 3},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v).HasLength(tc.l)
			assertSuccess(t, buf)
		})
	}
}

func Test_BaseAsserter_HasLength_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v1  any
		l   int
		out string
	}{
		// wrong length
		{v1: false, out: "value of type <bool> does not have a length"},
		{v1: any(false), out: "value of type <bool> does not have a length"},
		{v1: "ab", l: 3, out: "is not of length 3 (actual length = 2)"},
		{v1: map[string]any{"1": "abc"}, l: 3, out: "is not of length 3 (actual length = 1)"},
		{v1: map[string]any{"1": func() {}}, l: 3, out: "is not of length 3 (actual length = 1)"},
		{v1: [1]byte{1}, l: 3, out: "is not of length 3 (actual length = 1)"},
		{v1: []byte{1, 2}, l: 3, out: "is not of length 3 (actual length = 2)"},
		{v1: func() chan bool { c := make(chan bool, 123); c <- true; return c }(), l: 3, out: "is not of length 3 (actual length = 1)"},
		{v1: func() *string { c := "ab"; return &c }(), l: 3, out: "is not of length 3 (actual length = 2)"},
		{v1: func() **string { c := "ab"; d := &c; return &d }(), l: 3, out: "is not of length 3 (actual length = 2)"},
		// does not have a length
		{v1: func() **string { return nil }(), l: 3, out: "value of type <ptr> does not have a length"},
		{v1: true, l: 3, out: "value of type <bool> does not have a length"},
		{v1: math.Inf(1), l: 3, out: "value of type <float64> does not have a length"},
		{v1: 123, l: 3, out: "value of type <int> does not have a length"},
		{v1: 123.123, l: 3, out: "value of type <float64> does not have a length"},
		{v1: complex128(123), l: 3, out: "value of type <complex128> does not have a length"},
		{v1: math.NaN(), l: 3, out: "value of type <float64> does not have a length"},
		{v1: 123, l: 3, out: "value of type <int> does not have a length"},
		{v1: func() {}, l: 3, out: "value of type <func> does not have a length"},
		{v1: struct{ a bool }{true}, l: 3, out: "value of type <struct> does not have a length"},
		{v1: nil, l: 3, out: "value of type <invalid> does not have a length"},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v1).HasLength(tc.l)
			assertFail(t, buf, tc.out)
		})
	}
}

func Test_BaseAsserter_HasCapacity_Success(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v any
		c int
	}{
		{v: []byte(nil), c: 0},
		{v: make([]byte, 0, 16), c: 16},
		{v: [0]byte{}, c: 0},
		{v: (chan bool)(nil), c: 0},
		{v: make(chan bool, 16), c: 16},
		{v: []byte{1, 2, 3}, c: 3},
		{v: make([]byte, 3, 16), c: 16},
		{v: [3]byte{1, 2, 3}, c: 3},
		{v: func() chan bool { c := make(chan bool, 123); c <- true; c <- false; c <- true; return c }(), c: 123},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v).HasCapacity(tc.c)
			assertSuccess(t, buf)
		})
	}
}

func Test_BaseAsserter_HasCapacity_Fail(t *testing.T) {
	buf, fmtFn := newFormatSpy(t)
	base := assert.New(fmtFn)

	testCases := []struct {
		v1  any
		c   int
		out string
	}{
		// wrong capacity
		{v1: []byte{1, 2}, c: 3, out: "is not of capacity 3 (actual capacity = 2)"},
		{v1: [1]byte{1}, c: 3, out: "is not of capacity 3 (actual capacity = 1)"},
		{v1: func() chan bool { c := make(chan bool, 123); c <- true; return c }(), c: 3, out: "is not of capacity 3 (actual capacity = 123)"},
		// does not have capacity
		{v1: map[string]any{"1": "abc"}, c: 3, out: "value of type <map> does not have a capacity"},
		{v1: map[string]any{"1": func() {}}, c: 3, out: "value of type <map> does not have a capacity"},
		{v1: "ab", c: 3, out: "value of type <string> does not have a capacity"},
		{v1: func() *string { c := "ab"; return &c }(), c: 3, out: "value of type <string> does not have a capacity"},
		{v1: func() **string { c := "ab"; d := &c; return &d }(), c: 3, out: "value of type <ptr> does not have a capacity"},
		{v1: func() **string { return nil }(), c: 3, out: "value of type <ptr> does not have a capacity"},
		{v1: true, c: 3, out: "value of type <bool> does not have a capacity"},
		{v1: any(false), out: "value of type <bool> does not have a capacity"},
		{v1: math.Inf(1), c: 3, out: "value of type <float64> does not have a capacity"},
		{v1: 123, c: 3, out: "value of type <int> does not have a capacity"},
		{v1: 123.123, c: 3, out: "value of type <float64> does not have a capacity"},
		{v1: complex128(123), c: 3, out: "value of type <complex128> does not have a capacity"},
		{v1: math.NaN(), c: 3, out: "value of type <float64> does not have a capacity"},
		{v1: 123, c: 3, out: "value of type <int> does not have a capacity"},
		{v1: func() {}, c: 3, out: "value of type <func> does not have a capacity"},
		{v1: struct{ a bool }{true}, c: 3, out: "value of type <struct> does not have a capacity"},
		{v1: nil, c: 3, out: "value of type <invalid> does not have a capacity"},
	}
	for idx, tc := range testCases {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			base.Assert(tc.v1).HasCapacity(tc.c)
			assertFail(t, buf, tc.out)
		})
	}
}

func assertSuccess(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	t.Cleanup(func() {
		buf.Reset()
	})
	if buf.Len() > 0 {
		t.Fatalf("assertion failed, but was expect to succeed. got: %q", buf)
	}
}

func assertFail(t *testing.T, buf *bytes.Buffer, expected string) {
	t.Helper()
	t.Cleanup(func() {
		buf.Reset()
	})
	if buf.Len() == 0 {
		t.Fatalf("assertion succeeded, but was expected to fail.")
	}
	if actual := maskPtr(buf.String()); actual != expected {
		t.Fatalf("assertion error does not equal expected error.\n\tgot:  %q\n\twant: %q", actual, expected)
	}
}
