// Ring 0: raw checks only (see ring0_suite_test.go).
package assert_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

// EqualTestSuite covers the equality kernels and the shape of their messages.
type EqualTestSuite struct{}

func (s *EqualTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *EqualTestSuite) BeforeEach(t *gotest.T) *ring0Ctx { return &ring0Ctx{} }

func (s *EqualTestSuite) TestCheckEqual_EqualInts(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckEqual(42, 42), "equal ints")
}

func (s *EqualTestSuite) TestCheckEqual_EqualStrings(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckEqual("hello", "hello"), "equal strings")
}

func (s *EqualTestSuite) TestCheckEqual_EqualSlices(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckEqual([]int{1, 2, 3}, []int{1, 2, 3}), "equal slices")
}

func (s *EqualTestSuite) TestCheckEqual_EqualMaps(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckEqual(map[string]int{"a": 1, "b": 2}, map[string]int{"a": 1, "b": 2}), "equal maps")
}

func (s *EqualTestSuite) TestCheckEqual_NilSlices(t *gotest.T, _ *ring0Ctx) {
	var a, b []int
	mustPass(t, assert.CheckEqual(a, b), "nil slices")
}

func (s *EqualTestSuite) TestCheckEqual_EqualStructs(t *gotest.T, _ *ring0Ctx) {
	type Point struct{ X, Y int }
	mustPass(t, assert.CheckEqual(Point{1, 2}, Point{1, 2}), "equal structs")
}

func (s *EqualTestSuite) TestCheckEqual_UnequalInts(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckEqual(1, 2), "unequal ints", "Equal failed", "1", "2")
}

func (s *EqualTestSuite) TestCheckEqual_UnequalStrings(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckEqual("foo", "bar"), "unequal strings", "Equal failed", "foo", "bar")
}

// multilineValue's GoString returns multiline text, so CheckEqual produces a
// diff section for it.
type multilineValue struct {
	lines []string
}

func (m multilineValue) GoString() string {
	return strings.Join(m.lines, "\n")
}

func (s *EqualTestSuite) TestCheckEqual_UnequalMultilineValues_HasDiff(t *gotest.T, _ *ring0Ctx) {
	expected := multilineValue{[]string{"line one", "line two", "line three"}}
	actual := multilineValue{[]string{"line one", "line changed", "line three"}}
	mustFail(t, assert.CheckEqual(expected, actual), "unequal multiline values", "diff:", "- ", "+ ")
}

func (s *EqualTestSuite) TestCheckEqual_UnequalSingleLineStrings_NoDiff(t *gotest.T, _ *ring0Ctx) {
	result := assert.CheckEqual("hello", "world")
	mustFail(t, result, "unequal single-line strings")
	if strings.Contains(result, "diff:") {
		t.Errorf("single-line string mismatch should NOT have a diff section, got: %q", result)
	}
}

func (s *EqualTestSuite) TestCheckEqual_UnequalMaps(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckEqual(map[string]int{"a": 1}, map[string]int{"a": 2}), "unequal maps", "Equal failed")
}

func (s *EqualTestSuite) TestCheckEqual_NilVsNonNil(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckEqual(nil, 42), "nil vs non-nil", "Equal failed", "<nil>")
}

func (s *EqualTestSuite) TestCheckEqual_ErrorFormat(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckEqual(10, 20), "10 vs 20", "expected:", "actual:")
}

func (s *EqualTestSuite) TestCheckNotEqual_DifferentValues(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckNotEqual(1, 2), "different values")
}

func (s *EqualTestSuite) TestCheckNotEqual_DifferentStrings(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckNotEqual("foo", "bar"), "different strings")
}

func (s *EqualTestSuite) TestCheckNotEqual_DifferentSlices(t *gotest.T, _ *ring0Ctx) {
	mustPass(t, assert.CheckNotEqual([]int{1, 2}, []int{1, 3}), "different slices")
}

func (s *EqualTestSuite) TestCheckNotEqual_EqualValues(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckNotEqual(42, 42), "equal values", "NotEqual failed", "both are:", "42")
}

func (s *EqualTestSuite) TestCheckNotEqual_EqualStrings(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckNotEqual("hello", "hello"), "equal strings", "NotEqual failed", "hello")
}

func (s *EqualTestSuite) TestCheckNotEqual_EqualMaps(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckNotEqual(map[string]int{"x": 1}, map[string]int{"x": 1}), "equal maps", "NotEqual failed")
}

func (s *EqualTestSuite) TestCheckNotEqual_BothNil(t *gotest.T, _ *ring0Ctx) {
	mustFail(t, assert.CheckNotEqual(nil, nil), "both nil", "NotEqual failed", "<nil>")
}

type partlyHidden struct {
	Exported string
	Padding  string
	secret   int
}

func (s *EqualTestSuite) TestCheckEqual_UnexportedOnlyDifference_FallsBackToGoSyntax(t *gotest.T, _ *ring0Ctx) {
	a := partlyHidden{Exported: "long-enough-value-to-cross-the-threshold", Padding: "x", secret: 1}
	b := partlyHidden{Exported: "long-enough-value-to-cross-the-threshold", Padding: "x", secret: 2}
	mustFail(t, assert.CheckEqual(a, b), "unexported-only difference", "secret:1", "secret:2")
}
