// Ring 0: raw checks only (see ring0_suite_test.go).
package assert_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

// FormatTestSuite covers value formatting, diffs and message formatting.
type FormatTestSuite struct{}

func (s *FormatTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FormatTestSuite) BeforeEach(t *gotest.T) *ring0Ctx { return &ring0Ctx{} }

func mustEqualString(t *gotest.T, what, got, want string) {
	if got != want {
		fatalf(t, "%s = %q; want %q", what, got, want)
	}
}

func (s *FormatTestSuite) Test_FormatValue_Nil(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "FormatValue(nil)", assert.FormatValue(nil), "<nil>")
}

func (s *FormatTestSuite) Test_FormatValue_NilPointer(t *gotest.T, _ *ring0Ctx) {
	var p *string
	mustEqualString(t, "FormatValue((*string)(nil))", assert.FormatValue(p), "(*string)(nil)")
}

func (s *FormatTestSuite) Test_FormatValue_NonNilPointer(t *gotest.T, _ *ring0Ctx) {
	str := "hello"
	mustEqualString(t, "FormatValue(&s)", assert.FormatValue(&str), `"hello"`)
}

func (s *FormatTestSuite) Test_FormatValue_NonNilPointerInt(t *gotest.T, _ *ring0Ctx) {
	n := 42
	mustEqualString(t, "FormatValue(&n)", assert.FormatValue(&n), "42")
}

func (s *FormatTestSuite) Test_FormatValue_PlainValues(t *gotest.T, _ *ring0Ctx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		v    any
		want string
	}{
		{Desc: "true", v: true, want: "true"},
		{Desc: "false", v: false, want: "false"},
		{Desc: "int", v: 42, want: "42"},
		{Desc: "float", v: 3.14, want: "3.14"},
		{Desc: "string", v: "hello", want: `"hello"`},
		{Desc: "slice", v: []int{1, 2, 3}, want: "[]int{1, 2, 3}"},
		{Desc: "struct", v: struct{ A int }{A: 1}, want: "struct { A int }{A:1}"},
	}) {
		if got := assert.FormatValue(tc.v); got != tc.want {
			sub.Errorf("FormatValue(%v) = %q; want %q", tc.v, got, tc.want)
		}
	}
}

func (s *FormatTestSuite) Test_diff_IdenticalStrings(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "Diff(identical)", assert.Diff("hello", "hello"), "")
}

func (s *FormatTestSuite) Test_diff_BothSingleLine(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "Diff(both single-line)", assert.Diff("foo", "bar"), "")
}

func (s *FormatTestSuite) Test_diff_MultilineIdentical(t *gotest.T, _ *ring0Ctx) {
	str := "line1\nline2\nline3"
	mustEqualString(t, "Diff(identical multiline)", assert.Diff(str, str), "")
}

func (s *FormatTestSuite) Test_diff_MultilineAddedLine(t *gotest.T, _ *ring0Ctx) {
	got := assert.Diff("line1\nline2", "line1\nline2\nline3")
	mustFail(t, got, "diff with added line", "+ line3", "  line1", "  line2")
}

func (s *FormatTestSuite) Test_diff_MultilineRemovedLine(t *gotest.T, _ *ring0Ctx) {
	got := assert.Diff("line1\nline2\nline3", "line1\nline3")
	mustFail(t, got, "diff with removed line", "- line2")
}

func (s *FormatTestSuite) Test_diff_MultilineChangedLine(t *gotest.T, _ *ring0Ctx) {
	got := assert.Diff("line1\nlineA\nline3", "line1\nlineB\nline3")
	mustFail(t, got, "diff with changed line", "- lineA", "+ lineB")
}

func (s *FormatTestSuite) Test_FormatMessage_Empty(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "FormatMessage(nil)", assert.FormatMessage(nil), "")
	mustEqualString(t, "FormatMessage([])", assert.FormatMessage([]any{}), "")
}

func (s *FormatTestSuite) Test_FormatMessage_SingleString(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "FormatMessage([string])", assert.FormatMessage([]any{"hello world"}), "hello world")
}

func (s *FormatTestSuite) Test_FormatMessage_SingleNonString(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "FormatMessage([int])", assert.FormatMessage([]any{42}), "42")
}

func (s *FormatTestSuite) Test_FormatMessage_FormatString(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "FormatMessage([format, args...])",
		assert.FormatMessage([]any{"hello %s, you are %d years old", "Alice", 30}), "hello Alice, you are 30 years old")
}

func (s *FormatTestSuite) Test_FormatMessage_FormatStringNoArgs(t *gotest.T, _ *ring0Ctx) {
	// A lone string is a plain message, never a format.
	mustEqualString(t, "FormatMessage([plain string])", assert.FormatMessage([]any{"plain message"}), "plain message")
}

type wideOrder struct {
	ID     string
	Amount int
	Items  []string
	Region string
}

func (s *FormatTestSuite) TestFormatValueExpanded_SmallValuesStaySingleLine(t *gotest.T, _ *ring0Ctx) {
	if got := assert.FormatValueExpanded(map[string]int{"a": 1}); strings.Contains(got, "\n") {
		t.Errorf("small value should stay single-line, got %q", got)
	}
}

func (s *FormatTestSuite) TestFormatValueExpanded_LargeStructExpands(t *gotest.T, _ *ring0Ctx) {
	v := wideOrder{ID: "A-0001", Amount: 100, Items: []string{"alpha", "beta"}, Region: "eu-central-1"}
	got := assert.FormatValueExpanded(v)
	if !strings.Contains(got, "\n") {
		fatalf(t, "large struct should expand to multiline, got %q", got)
	}
	if !strings.Contains(got, "  Amount: 100,") {
		t.Errorf("expanded struct should have one field per line, got %q", got)
	}
}

func (s *FormatTestSuite) TestFormatValueExpanded_MapKeysSorted(t *gotest.T, _ *ring0Ctx) {
	v := map[string]string{"zebra": "last", "alpha": "first", "middle": "mid-entry-padding"}
	got := assert.FormatValueExpanded(v)
	ia, iz := strings.Index(got, `"alpha"`), strings.Index(got, `"zebra"`)
	if ia < 0 || iz < 0 || ia > iz {
		t.Errorf("map keys should be sorted deterministically, got %q", got)
	}
}

func (s *FormatTestSuite) TestCheckEqualViaExpansion_LargeStructsGetDiff(t *gotest.T, _ *ring0Ctx) {
	want := wideOrder{ID: "A-0001", Amount: 100, Items: []string{"alpha", "beta"}, Region: "eu-central-1"}
	got := wideOrder{ID: "A-0001", Amount: 150, Items: []string{"alpha"}, Region: "eu-central-1"}
	mustFail(t, assert.CheckEqual(want, got), "large-struct inequality", "diff:", "- ", "+ ")
}

func (s *FormatTestSuite) TestFormatValueExpanded_HugeValuesStaySingleLine(t *gotest.T, _ *ring0Ctx) {
	huge := make([]int, 20000)
	for i := range huge {
		huge[i] = i
	}
	other := make([]int, 20000)
	copy(other, huge)
	other[19999] = -1
	if msg := assert.CheckEqual(huge, other); strings.Contains(msg, "diff:") {
		t.Errorf("huge values must not expand into a quadratic diff")
	}
}
