// These tests verify the assertion primitives that gotest is built on. Using
// gotest suites here would be circular: a bug in the assertion logic would
// silently pass its own tests, making failures undetectable. stdlib tests with
// raw if/t.Error are the only way to independently verify correctness at this
// layer.
package assert //nolint:stdlib-test

import (
	"strings"
	"testing"
)

// ---- CheckEqual tests ----

func TestCheckEqual_EqualInts(t *testing.T) {
	result := CheckEqual(42, 42)
	if result != "" {
		t.Errorf("expected empty string for equal ints, got: %q", result)
	}
}

func TestCheckEqual_EqualStrings(t *testing.T) {
	result := CheckEqual("hello", "hello")
	if result != "" {
		t.Errorf("expected empty string for equal strings, got: %q", result)
	}
}

func TestCheckEqual_EqualSlices(t *testing.T) {
	result := CheckEqual([]int{1, 2, 3}, []int{1, 2, 3})
	if result != "" {
		t.Errorf("expected empty string for equal slices, got: %q", result)
	}
}

func TestCheckEqual_EqualMaps(t *testing.T) {
	result := CheckEqual(map[string]int{"a": 1, "b": 2}, map[string]int{"a": 1, "b": 2})
	if result != "" {
		t.Errorf("expected empty string for equal maps, got: %q", result)
	}
}

func TestCheckEqual_NilSlices(t *testing.T) {
	var a []int
	var b []int
	result := CheckEqual(a, b)
	if result != "" {
		t.Errorf("expected empty string for nil slices, got: %q", result)
	}
}

func TestCheckEqual_EqualStructs(t *testing.T) {
	type Point struct{ X, Y int }
	result := CheckEqual(Point{1, 2}, Point{1, 2})
	if result != "" {
		t.Errorf("expected empty string for equal structs, got: %q", result)
	}
}

func TestCheckEqual_UnequalInts(t *testing.T) {
	result := CheckEqual(1, 2)
	if result == "" {
		t.Fatal("expected non-empty error string for unequal ints")
	}
	if !strings.Contains(result, "Equal failed") {
		t.Errorf("error should contain 'Equal failed', got: %q", result)
	}
	if !strings.Contains(result, "1") {
		t.Errorf("error should contain expected value '1', got: %q", result)
	}
	if !strings.Contains(result, "2") {
		t.Errorf("error should contain actual value '2', got: %q", result)
	}
}

func TestCheckEqual_UnequalStrings(t *testing.T) {
	result := CheckEqual("foo", "bar")
	if result == "" {
		t.Fatal("expected non-empty error string for unequal strings")
	}
	if !strings.Contains(result, "Equal failed") {
		t.Errorf("error should contain 'Equal failed', got: %q", result)
	}
	if !strings.Contains(result, "foo") {
		t.Errorf("error should contain expected value 'foo', got: %q", result)
	}
	if !strings.Contains(result, "bar") {
		t.Errorf("error should contain actual value 'bar', got: %q", result)
	}
}

// multilineValue is a test helper whose GoString() returns multiline text,
// allowing CheckEqual to produce a diff section in its output.
type multilineValue struct {
	lines []string
}

func (m multilineValue) GoString() string {
	return strings.Join(m.lines, "\n")
}

func TestCheckEqual_UnequalMultilineValues_HasDiff(t *testing.T) {
	expected := multilineValue{[]string{"line one", "line two", "line three"}}
	actual := multilineValue{[]string{"line one", "line changed", "line three"}}
	result := CheckEqual(expected, actual)
	if result == "" {
		t.Fatal("expected non-empty error string for unequal multiline values")
	}
	if !strings.Contains(result, "diff:") {
		t.Errorf("error should contain 'diff:' section for multiline values, got: %q", result)
	}
	if !strings.Contains(result, "- ") {
		t.Errorf("error diff should contain removed lines (- prefix), got: %q", result)
	}
	if !strings.Contains(result, "+ ") {
		t.Errorf("error diff should contain added lines (+ prefix), got: %q", result)
	}
}

func TestCheckEqual_UnequalSingleLineStrings_NoDiff(t *testing.T) {
	result := CheckEqual("hello", "world")
	if result == "" {
		t.Fatal("expected non-empty error string for unequal single-line strings")
	}
	if strings.Contains(result, "diff:") {
		t.Errorf("single-line string mismatch should NOT have diff section, got: %q", result)
	}
}

func TestCheckEqual_UnequalMaps(t *testing.T) {
	result := CheckEqual(map[string]int{"a": 1}, map[string]int{"a": 2})
	if result == "" {
		t.Fatal("expected non-empty error string for unequal maps")
	}
	if !strings.Contains(result, "Equal failed") {
		t.Errorf("error should contain 'Equal failed', got: %q", result)
	}
}

func TestCheckEqual_NilVsNonNil(t *testing.T) {
	result := CheckEqual(nil, 42)
	if result == "" {
		t.Fatal("expected non-empty error string for nil vs non-nil")
	}
	if !strings.Contains(result, "Equal failed") {
		t.Errorf("error should contain 'Equal failed', got: %q", result)
	}
	if !strings.Contains(result, "<nil>") {
		t.Errorf("error should contain '<nil>' for nil value, got: %q", result)
	}
}

func TestCheckEqual_ErrorFormat(t *testing.T) {
	result := CheckEqual(10, 20)
	if !strings.Contains(result, "expected:") {
		t.Errorf("error should contain 'expected:' label, got: %q", result)
	}
	if !strings.Contains(result, "actual:") {
		t.Errorf("error should contain 'actual:' label, got: %q", result)
	}
}

// ---- CheckNotEqual tests ----

func TestCheckNotEqual_DifferentValues(t *testing.T) {
	result := CheckNotEqual(1, 2)
	if result != "" {
		t.Errorf("expected empty string for different values, got: %q", result)
	}
}

func TestCheckNotEqual_DifferentStrings(t *testing.T) {
	result := CheckNotEqual("foo", "bar")
	if result != "" {
		t.Errorf("expected empty string for different strings, got: %q", result)
	}
}

func TestCheckNotEqual_DifferentSlices(t *testing.T) {
	result := CheckNotEqual([]int{1, 2}, []int{1, 3})
	if result != "" {
		t.Errorf("expected empty string for different slices, got: %q", result)
	}
}

func TestCheckNotEqual_EqualValues(t *testing.T) {
	result := CheckNotEqual(42, 42)
	if result == "" {
		t.Fatal("expected non-empty error string for equal values")
	}
	if !strings.Contains(result, "NotEqual failed") {
		t.Errorf("error should contain 'NotEqual failed', got: %q", result)
	}
	if !strings.Contains(result, "both are:") {
		t.Errorf("error should contain 'both are:', got: %q", result)
	}
	if !strings.Contains(result, "42") {
		t.Errorf("error should contain the value '42', got: %q", result)
	}
}

func TestCheckNotEqual_EqualStrings(t *testing.T) {
	result := CheckNotEqual("hello", "hello")
	if result == "" {
		t.Fatal("expected non-empty error string for equal strings")
	}
	if !strings.Contains(result, "NotEqual failed") {
		t.Errorf("error should contain 'NotEqual failed', got: %q", result)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("error should contain the value 'hello', got: %q", result)
	}
}

func TestCheckNotEqual_EqualMaps(t *testing.T) {
	result := CheckNotEqual(map[string]int{"x": 1}, map[string]int{"x": 1})
	if result == "" {
		t.Fatal("expected non-empty error string for equal maps")
	}
	if !strings.Contains(result, "NotEqual failed") {
		t.Errorf("error should contain 'NotEqual failed', got: %q", result)
	}
}

func TestCheckNotEqual_BothNil(t *testing.T) {
	result := CheckNotEqual(nil, nil)
	if result == "" {
		t.Fatal("expected non-empty error string for both nil")
	}
	if !strings.Contains(result, "NotEqual failed") {
		t.Errorf("error should contain 'NotEqual failed', got: %q", result)
	}
	if !strings.Contains(result, "<nil>") {
		t.Errorf("error should contain '<nil>', got: %q", result)
	}
}
