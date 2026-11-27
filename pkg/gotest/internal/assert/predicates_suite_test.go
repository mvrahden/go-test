// Ring 0: raw checks only (see ring0_suite_test.go). The predicates decide
// what counts as nil, empty, contained, panicking, a length or equal JSON for
// every assertion that asks; a wrong answer here passes tests that should fail.
package assert_test //nolint:fail-guard // ring 0: raw checks only, never a gotest assertion

import (
	"encoding/json"
	"io"
	"math"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type PredicatesTestSuite struct{}

func (s *PredicatesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type predicatesCtx struct{}

func (s *PredicatesTestSuite) BeforeEach(t *gotest.T) *predicatesCtx { return &predicatesCtx{} }

func (s *PredicatesTestSuite) TestIsNil(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	var nilPtr *int
	var nilMap map[string]int
	var nilFn func()
	v := 1
	for _, c := range []struct {
		what string
		in   any
		want bool
	}{
		{"untyped nil", nil, true},
		{"nil pointer", nilPtr, true},
		{"nil map", nilMap, true},
		{"nil func", nilFn, true},
		{"non-nil pointer", &v, false},
		{"empty slice is not nil", []int{}, false},
		{"int is never nil", 0, false},
		{"empty string is never nil", "", false},
	} {
		if got := assert.IsNil(c.in); got != c.want {
			t.Errorf("IsNil(%s): got %v, want %v", c.what, got, c.want)
		}
	}
}

func (s *PredicatesTestSuite) TestIsNilable(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	v := 1
	for _, c := range []struct {
		what string
		in   any
		want bool
	}{
		{"pointer", &v, true},
		{"slice", []int{1}, true},
		{"map", map[string]int{}, true},
		{"chan", make(chan int), true},
		{"func", func() {}, true},
		{"int", 1, false},
		{"string", "x", false},
		{"struct", struct{}{}, false},
	} {
		if got := assert.IsNilable(c.in); got != c.want {
			t.Errorf("IsNilable(%s): got %v, want %v", c.what, got, c.want)
		}
	}
}

func (s *PredicatesTestSuite) TestIsEmpty(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	empty := []int{}
	full := []int{1}
	zero := 0
	ptrToEmpty := &empty
	for _, c := range []struct {
		what string
		in   any
		want bool
	}{
		{"untyped nil", nil, true},
		{"nil slice", []int(nil), true},
		{"empty slice", empty, true},
		{"one element", full, false},
		{"empty string", "", true},
		{"string", "a", false},
		{"empty map", map[string]int{}, true},
		{"map with an entry", map[string]int{"a": 1}, false},
		{"empty array", [0]int{}, true},
		{"array", [1]int{1}, false},
		{"nil pointer", (*[]int)(nil), true},
		{"pointer to empty", &empty, true},
		{"pointer to one element", &full, false},
		{"int is never empty", 0, false},
	} {
		if got := assert.IsEmpty(c.in); got != c.want {
			t.Errorf("IsEmpty(%s): got %v, want %v", c.what, got, c.want)
		}
	}
	for _, c := range []struct {
		what string
		in   any
		want bool
	}{
		{"slice", []int{}, true},
		{"string", "", true},
		{"pointer", &empty, true},
		{"chan", make(chan int), true},
		{"int", 0, false},
		{"struct", struct{}{}, false},
		{"pointer to an int", &zero, false},
		{"pointer to a struct", &struct{}{}, false},
		{"nil pointer to an int", (*int)(nil), true},
		{"pointer to a pointer to a slice", &ptrToEmpty, true},
	} {
		if got := assert.IsEmptyable(c.in); got != c.want {
			t.Errorf("IsEmptyable(%s): got %v, want %v", c.what, got, c.want)
		}
	}
}

func (s *PredicatesTestSuite) TestIncludesElement(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	type named string
	for _, c := range []struct {
		what       string
		s, element any
		found      bool
		guarded    bool // the pair is one the question cannot be asked of
	}{
		{"substring present", "abc", "b", true, false},
		{"substring absent", "abc", "z", false, false},
		{"substring of a named kind", named("abc"), named("b"), true, false},
		{"non-string needle in a string", "abc", 1, false, true},
		{"slice element present", []int{1, 2}, 2, true, false},
		{"slice element absent", []int{1, 2}, 3, false, false},
		{"slice element of a type it cannot hold", []int{1, 2}, "x", false, true},
		{"any-slice holds every type", []any{1, "x"}, "x", true, false},
		{"array element present", [2]string{"a", "b"}, "a", true, false},
		{"deep-equal element", []struct{ N int }{{1}}, struct{ N int }{1}, true, false},
		{"map key present", map[string]int{"k": 1}, "k", true, false},
		{"map key absent", map[string]int{"k": 1}, "z", false, false},
		{"map key of the wrong type", map[string]int{"k": 1}, 1, false, true},
		{"nil key in an any-keyed map", map[any]int{nil: 1}, nil, true, false},
		{"nil key absent from an any-keyed map", map[any]int{"k": 1}, nil, false, false},
		{"nil key in a pointer-keyed map", map[*int]int{nil: 1}, nil, true, false},
		{"nil key in a string-keyed map", map[string]int{"": 1}, nil, false, true},
		{"an interface element type the needle does not implement", []error{io.EOF}, "EOF", false, true},
		{"an interface element type the needle implements", []error{io.EOF}, io.EOF, true, false},
		{"nil element in a pointer slice", []*int{nil}, nil, true, false},
		{"nil element absent from a pointer slice", []*int{new(int)}, nil, false, false},
		{"nil element in an any-slice", []any{1, nil}, nil, true, false},
		{"nil element in a slice of slices", [][]int{nil}, nil, true, false},
		{"nil element in an int slice", []int{1}, nil, false, true},
		{"nil element in an int array", [1]int{}, nil, false, true},
		{"unhashable key in an any-keyed map", map[any]int{1: 1}, []int{1}, false, true},
		{"key holding an unhashable value in an any-keyed map", map[any]int{1: 1}, struct{ V any }{[]int{1}}, false, true},
		{"nil haystack", nil, "a", false, false},
		{"int haystack is invalid", 42, 4, false, true},
		{"struct haystack is invalid", struct{}{}, "a", false, true},
	} {
		found, guard := assert.IncludesElement(c.s, c.element)
		if found != c.found || (guard != "") != c.guarded {
			t.Errorf("IncludesElement(%s): got (%v, %q), want (%v, guarded=%v)", c.what, found, guard, c.found, c.guarded)
		}
	}
}

func (s *PredicatesTestSuite) TestIncludesElementGuards(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	type named string
	for _, c := range []struct {
		s, element any
		guard      string
	}{
		{[]int64{1}, 3, "[]int64 cannot hold a value of type int (convert it to int64)"},
		{[]int{1}, "x", "[]int cannot hold a value of type string"},
		{[]named{"a"}, "a", "[]assert_test.named cannot hold a value of type string (convert it to assert_test.named)"},
		{[]error{io.EOF}, "EOF", "[]error cannot hold a value of type string"},
		{[]int{1}, nil, "[]int cannot hold a nil element"},
		{"abc", 1, "string cannot hold a value of type int"},
		{"abc", 'b', "string cannot hold a value of type int32 (convert it to string)"},
		{"abc", byte('b'), "string cannot hold a value of type uint8 (convert it to string)"},
		{"abc", []byte("b"), "string cannot hold a value of type []uint8 (convert it to string)"},
		{named("abc"), 'b', "assert_test.named cannot hold a value of type int32 (convert it to assert_test.named)"},
		{"abc", nil, "string cannot hold a nil substring"},
		{[]int{3}, 3.0, "[]int cannot hold a value of type float64 (convert it to int)"},
		{[]int{3}, 3.7, "[]int cannot hold a value of type float64"},
		{[]int8{1}, 7, "[]int8 cannot hold a value of type int (convert it to int8)"},
		{[]int8{1}, 300, "[]int8 cannot hold a value of type int"},
		{[]uint{1}, -1, "[]uint cannot hold a value of type int"},
		{[]float64{1}, 1<<53 + 1, "[]float64 cannot hold a value of type int"},
		{[]float64{1}, 2, "[]float64 cannot hold a value of type int (convert it to float64)"},
		{[]int64{1}, uint64(1 << 63), "[]int64 cannot hold a value of type uint64"},
		{map[int8]int{1: 1}, 300, "map[int8]int cannot hold a key of type int"},
		{map[int64]int{1: 1}, 7, "map[int64]int cannot hold a key of type int (convert it to int64)"},
		{map[string]int{"a": 1}, 7, "map[string]int cannot hold a key of type int"},
		{map[string]int{"": 1}, nil, "map[string]int cannot hold a nil key"},
		{map[any]int{1: 1}, []int{1}, "map[interface {}]int cannot hold a key of type []int: it is not comparable"},
	} {
		_, guard := assert.IncludesElement(c.s, c.element)
		if guard != c.guard {
			t.Errorf("IncludesElement(%T, %T): guard %q, want %q", c.s, c.element, guard, c.guard)
		}
	}
}

func (s *PredicatesTestSuite) TestDidPanic(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	recovered, panicked := assert.DidPanic(func() { panic("boom") })
	if !panicked || recovered != "boom" {
		t.Errorf("a panicking func: got (%v, %v), want (boom, true)", recovered, panicked)
	}
	recovered, panicked = assert.DidPanic(func() {})
	if panicked || recovered != nil {
		t.Errorf("a quiet func: got (%v, %v), want (nil, false)", recovered, panicked)
	}
}

func (s *PredicatesTestSuite) TestNormalizeJSON(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	want := map[string]any{"a": json.Number("1")} // numbers stay as written
	for _, c := range []struct {
		what string
		in   any
	}{
		{"string", `{"a":1}`},
		{"bytes", []byte(`{"a": 1}`)},
		{"raw message", json.RawMessage(`{"a":1}`)},
		{"reader", strings.NewReader(`{ "a" : 1 }`)},
		{"marshalable value", map[string]int{"a": 1}},
	} {
		got, err := assert.NormalizeJSON(c.in)
		if err != nil {
			t.Errorf("NormalizeJSON(%s): %v", c.what, err)
			continue
		}
		m, ok := got.(map[string]any)
		if !ok || len(m) != 1 || m["a"] != want["a"] {
			t.Errorf("NormalizeJSON(%s): got %#v, want %#v", c.what, got, want)
		}
	}
	if _, err := assert.NormalizeJSON("not json"); err == nil {
		t.Errorf("NormalizeJSON(invalid): expected an error")
	}
	if _, err := assert.NormalizeJSON(func() {}); err == nil {
		t.Errorf("NormalizeJSON(unmarshalable): expected an error")
	}
	for _, in := range []string{`{"a":1}}`, `[1]]`, `{"a":1} 2`, `1 ]`} {
		if _, err := assert.NormalizeJSON(in); err == nil {
			t.Errorf("NormalizeJSON(%s): trailing data must be an error", in)
		}
	}
	if _, err := assert.NormalizeJSON("{\"a\":1}\n \t"); err != nil {
		t.Errorf("NormalizeJSON(trailing whitespace): %v", err)
	}
	if got, err := assert.NormalizeJSON(jsonText(`{"a":1}`)); err != nil {
		t.Errorf("NormalizeJSON(named string): %v", err)
	} else if m, ok := got.(map[string]any); !ok || m["a"] != want["a"] {
		t.Errorf("NormalizeJSON(named string): got %#v, want %#v", got, want)
	}
	if _, err := assert.NormalizeJSON(jsonText("{not json")); err == nil || !strings.Contains(err.Error(), "is read as JSON text, as a string is; marshal it to compare it as a value") {
		t.Errorf("NormalizeJSON(invalid named string): got %v, want the hint", err)
	}
	if got, err := assert.NormalizeJSON(shout("a")); err != nil || got != "A" {
		t.Errorf("NormalizeJSON(named string with MarshalJSON): got %#v, %v, want \"A\"", got, err)
	}
	for _, in := range []any{"\"a\xff\"", []byte("\"a\xff\""), jsonText("\"a\xff\"")} {
		if _, err := assert.NormalizeJSON(in); err == nil || !strings.Contains(err.Error(), "invalid UTF-8 at byte 2") {
			t.Errorf("NormalizeJSON(%T with an invalid byte): got %v, want invalid UTF-8 at byte 2", in, err)
		}
	}
}

// jsonText is JSON text of a named string type.
type jsonText string

// shout is a named string that marshals itself.
type shout string

func (s shout) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strings.ToUpper(string(s)) + `"`), nil
}

func (s *PredicatesTestSuite) TestReadAndRestore(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	r := strings.NewReader("payload")
	b, err := assert.ReadAndRestore(r)
	if err != nil || string(b) != "payload" {
		t.Errorf("ReadAndRestore: got (%q, %v), want (payload, nil)", b, err)
	}
	rest, _ := assert.ReadAndRestore(r)
	if string(rest) != "payload" {
		t.Errorf("a seekable reader must be rewound after reading: second read got %q", rest)
	}
	nr := io.NopCloser(strings.NewReader("ephemeral"))
	b, err = assert.ReadAndRestore(nr)
	if err != nil || string(b) != "ephemeral" {
		t.Errorf("ReadAndRestore(non-seekable): got (%q, %v), want (ephemeral, nil)", b, err)
	}
	if rest, _ := io.ReadAll(nr); len(rest) != 0 {
		t.Errorf("a non-seekable reader is consumed: second read got %q", rest)
	}
}

func (s *PredicatesTestSuite) TestLength(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	arr := [2]int{1, 2}
	for _, c := range []struct {
		what string
		in   any
		n    int
		ok   bool
	}{
		{"slice", []int{1, 2, 3}, 3, true},
		{"nil slice", []int(nil), 0, true},
		{"string", "hello", 5, true},
		{"map", map[string]int{"a": 1, "b": 2}, 2, true},
		{"nil map", map[string]int(nil), 0, true},
		{"array", [3]int{1, 2, 3}, 3, true},
		{"buffered channel counts queued elements", ch, 2, true},
		{"untyped nil has no length", nil, 0, false},
		{"pointer to array has no length", &arr, 0, false},
		{"nil pointer has no length", (*[]int)(nil), 0, false},
		{"int has no length", 42, 0, false},
		{"struct has no length", struct{}{}, 0, false},
	} {
		n, ok := assert.Length(c.in)
		if n != c.n || ok != c.ok {
			t.Errorf("Length(%s): got (%d, %v), want (%d, %v)", c.what, n, ok, c.n, c.ok)
		}
	}
}

func (s *PredicatesTestSuite) TestJSONEqual(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	norm := func(v any) any {
		out, err := assert.NormalizeJSON(v)
		if err != nil {
			t.Errorf("NormalizeJSON(%v): %v", v, err)
		}
		return out
	}
	for _, c := range []struct {
		what             string
		expected, actual any
		want             bool
	}{
		{"key order is ignored", `{"a":1,"b":2}`, `{"b":2,"a":1}`, true},
		{"nested key order is ignored", `{"a":{"x":1,"y":2}}`, `{"a":{"y":2,"x":1}}`, true},
		{"a struct equals its encoding", struct {
			A int `json:"a"`
		}{5}, `{"a":5}`, true},
		{"equal arrays", `[1,2,3]`, `[1,2,3]`, true},
		{"different values", `{"a":1}`, `{"a":2}`, false},
		{"different keys", `{"a":1}`, `{"b":1}`, false},
		{"null is not an empty object", `null`, `{}`, false},
		{"array order matters", `[1,2,3]`, `[3,2,1]`, false},
		{"an exponent past a rational's reach", `1e10000000`, `10e9999999`, true},
		{"exponents past a rational's reach differ", `1e10000000`, `1e10000001`, false},
		{"an exponent past int64", `5e99999999999999999999`, `50e99999999999999999998`, true},
		{"a fraction and its exponent form", `0.1`, `1e-1`, true},
		{"an exponent in capitals with a sign", `1E+2`, `100`, true},
		{"trailing zeros of a fraction", `2.50`, `2.5`, true},
		{"negative zero is zero", `-0`, `0`, true},
		{"negative zero with exponent is zero", `-0.0e5`, `0`, true},
		{"sign matters", `-1`, `1`, false},
		{"close values differ", `1.0000000000000000000001`, `1`, false},
	} {
		if got := assert.JSONEqual(norm(c.expected), norm(c.actual)); got != c.want {
			t.Errorf("JSONEqual(%s): got %v, want %v", c.what, got, c.want)
		}
	}
}

// nilErr is an error whose Error method reads through its receiver.
type nilErr struct{ msg string }

func (e *nilErr) Error() string { return e.msg }

func (s *PredicatesTestSuite) TestErrorText(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	if got := assert.ErrorText(io.EOF); got != "EOF" {
		t.Errorf("ErrorText(io.EOF): got %q", got)
	}
	var typedNil *nilErr
	got := assert.ErrorText(typedNil)
	for _, want := range []string{"*assert_test.nilErr(nil), a nil pointer in a non-nil error; its Error method panicked:", "nil pointer dereference"} {
		if !strings.Contains(got, want) {
			t.Errorf("ErrorText(typed nil): got %q, want it to hold %q", got, want)
		}
	}
}

func (s *PredicatesTestSuite) TestCheckElementsMatch_Unhashable(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	a, b := []any{[]int{1}, "x"}, []any{"x", []int{1}}
	const want = "an element of type []int cannot be compared with ==; compare such lists with Equal"
	if got := assert.CheckElementsMatch(a, b); !strings.Contains(got, "ElementsMatch failed:") || !strings.Contains(got, want) {
		t.Errorf("CheckElementsMatch(unhashable): got %q", got)
	}
	if got := assert.CheckSubset(a, b); !strings.Contains(got, "Subset failed:") || !strings.Contains(got, want) {
		t.Errorf("CheckSubset(unhashable): got %q", got)
	}
	if got := assert.CheckElementsMatch([]any{1, "x"}, []any{"x", 1}); got != "" {
		t.Errorf("CheckElementsMatch(hashable any): got %q", got)
	}
	if got := assert.CheckElementsMatch([]int{1, 1, 2}, []int{1, 2, 2}); !strings.Contains(got, "is in listB but not enough times in listA") {
		t.Errorf("CheckElementsMatch(duplicate counts): got %q", got)
	}
}

func (s *PredicatesTestSuite) TestCheckElementsMatch_NaN(t *gotest.T, _ *predicatesCtx) {
	t.AssertsBy(rawCheck)
	type pt struct{ X, Y float64 }
	nan := math.NaN()
	msg := func(v, list string) string {
		return "ElementsMatch failed:\n  element " + v + " in " + list + " is not equal to itself: a NaN matches nothing, itself included"
	}
	if got, want := assert.CheckElementsMatch([]float64{1, 2}, []float64{nan, nan}), msg("NaN", "listB"); got != want {
		t.Errorf("CheckElementsMatch(NaNs in listB): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]float64{nan, nan}, []float64{1, 2}), msg("NaN", "listA"); got != want {
		t.Errorf("CheckElementsMatch(NaNs in listA): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]float64{nan}, []float64{nan}), msg("NaN", "listA"); got != want {
		t.Errorf("CheckElementsMatch(a NaN in both): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]float64{0.5, 0.25}, []float64{0.5, nan}), msg("NaN", "listB"); got != want {
		t.Errorf("CheckElementsMatch(a NaN after a match): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]float32{1, 2}, []float32{float32(nan), float32(nan)}), msg("NaN", "listB"); got != want {
		t.Errorf("CheckElementsMatch(float32 NaNs): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]pt{{1, 2}}, []pt{{nan, 7}}), msg("assert_test.pt{X:NaN, Y:7}", "listB"); got != want {
		t.Errorf("CheckElementsMatch(a struct holding a NaN): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([][2]float64{{1, 2}}, [][2]float64{{nan, 7}}), msg("[2]float64{NaN, 7}", "listB"); got != want {
		t.Errorf("CheckElementsMatch(an array holding a NaN): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]any{1.0}, []any{nan}), msg("NaN", "listB"); got != want {
		t.Errorf("CheckElementsMatch(an any holding a NaN): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]any{nan, []int{1}}, []any{nan, []int{1}}), msg("NaN", "listA"); got != want {
		t.Errorf("CheckElementsMatch(a NaN before a slice): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]any{[]int{1}, nan}, []any{[]int{1}, nan}), "ElementsMatch failed:\n  an element of type []int cannot be compared with ==; compare such lists with Equal"; got != want {
		t.Errorf("CheckElementsMatch(a slice before a NaN): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]float64{nan}, []float64{1, 2}), "ElementsMatch failed:\n  lists have different lengths: 1 vs 2"; got != want {
		t.Errorf("CheckElementsMatch(a NaN, lengths differ): got %q, want %q", got, want)
	}
	if got, want := assert.CheckElementsMatch([]float64{1, 2, 2}, []float64{1, 1, 2}), "ElementsMatch failed:\n  element 1 is in listB but not enough times in listA"; got != want {
		t.Errorf("CheckElementsMatch(counts differ): got %q, want %q", got, want)
	}
	if got := assert.CheckElementsMatch([]float64{1, 2, 2}, []float64{2, 1, 2}); got != "" {
		t.Errorf("CheckElementsMatch(reordered floats): got %q, want \"\"", got)
	}
	if got := assert.CheckElementsMatch([]float64{math.Inf(1), 0}, []float64{0, math.Inf(1)}); got != "" {
		t.Errorf("CheckElementsMatch(infinities): got %q, want \"\"", got)
	}
	if got := assert.CheckElementsMatch([]float64{}, nil); got != "" {
		t.Errorf("CheckElementsMatch(empty and nil): got %q, want \"\"", got)
	}
	if got, want := assert.CheckSubset([]float64{1, nan}, []float64{nan}), "Subset failed:\n  element NaN from subset not found in list"; got != want {
		t.Errorf("CheckSubset(a NaN in subset): got %q, want %q", got, want)
	}
	if got := assert.CheckSubset([]float64{1, nan}, []float64{1}); got != "" {
		t.Errorf("CheckSubset(a NaN in list): got %q, want \"\"", got)
	}
}
