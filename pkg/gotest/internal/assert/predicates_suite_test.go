// Ring 0: raw checks only (see ring0_suite_test.go). The predicates decide
// what counts as nil, empty, contained, panicking, a length or equal JSON for
// every assertion that asks; a wrong answer here passes tests that should fail.
package assert_test //nolint:fail-guard

import (
	"encoding/json"
	"io"
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
	empty := []int{}
	full := []int{1}
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
	} {
		if got := assert.IsEmptyable(c.in); got != c.want {
			t.Errorf("IsEmptyable(%s): got %v, want %v", c.what, got, c.want)
		}
	}
}

func (s *PredicatesTestSuite) TestIncludesElement(t *gotest.T, _ *predicatesCtx) {
	for _, c := range []struct {
		what       string
		s, element any
		found      bool
		valid      bool
	}{
		{"substring present", "abc", "b", true, true},
		{"substring absent", "abc", "z", false, true},
		{"non-string needle in a string", "abc", 1, false, true},
		{"slice element present", []int{1, 2}, 2, true, true},
		{"slice element absent", []int{1, 2}, 3, false, true},
		{"array element present", [2]string{"a", "b"}, "a", true, true},
		{"deep-equal element", []struct{ N int }{{1}}, struct{ N int }{1}, true, true},
		{"map key present", map[string]int{"k": 1}, "k", true, true},
		{"map key absent", map[string]int{"k": 1}, "z", false, true},
		{"map key of the wrong type", map[string]int{"k": 1}, 1, false, true},
		{"nil haystack", nil, "a", false, true},
		{"int haystack is invalid", 42, 4, false, false},
		{"struct haystack is invalid", struct{}{}, "a", false, false},
	} {
		found, valid := assert.IncludesElement(c.s, c.element)
		if found != c.found || valid != c.valid {
			t.Errorf("IncludesElement(%s): got (%v, %v), want (%v, %v)", c.what, found, valid, c.found, c.valid)
		}
	}
}

func (s *PredicatesTestSuite) TestDidPanic(t *gotest.T, _ *predicatesCtx) {
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
	want := map[string]any{"a": float64(1)}
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
}

func (s *PredicatesTestSuite) TestReadAndRestore(t *gotest.T, _ *predicatesCtx) {
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
	} {
		if got := assert.JSONEqual(norm(c.expected), norm(c.actual)); got != c.want {
			t.Errorf("JSONEqual(%s): got %v, want %v", c.what, got, c.want)
		}
	}
}
