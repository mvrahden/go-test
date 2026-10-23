// Ring 0: raw checks only (see ring0_suite_test.go).
package assert_test //nolint:fail-guard

import (
	"strings"
	"sync"
	"time"

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
	mustEqualString(t, "Diff(added line)", assert.Diff("line1\nline2", "line1\nline2\nline3"), " line1\n line2\n+line3\n")
}

func (s *FormatTestSuite) Test_diff_MultilineRemovedLine(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "Diff(removed line)", assert.Diff("line1\nline2\nline3", "line1\nline3"), " line1\n-line2\n line3\n")
}

func (s *FormatTestSuite) Test_diff_MultilineChangedLine(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "Diff(changed line)", assert.Diff("line1\nlineA\nline3", "line1\nlineB\nline3"), " line1\n-lineA\n+lineB\n line3\n")
}

func (s *FormatTestSuite) Test_diff_FinalNewlineOnOneSide(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "Diff(final newline)", assert.Diff("a\nb\n", "a\nb"), " a\n-b\n+b\n\\ No newline at end of file\n")
}

func (s *FormatTestSuite) Test_diff_NoFinalNewlineOnEitherSide(t *gotest.T, _ *ring0Ctx) {
	mustEqualString(t, "Diff(no final newlines)", assert.Diff("a\nb", "a\nc"), " a\n-b\n+c\n")
}

func (s *FormatTestSuite) Test_diff_DistantChanges_SeparatedWithoutHeaders(t *gotest.T, _ *ring0Ctx) {
	before := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	after := "one\n2\n3\n4\n5\n6\n7\n8\n9\nten\n"
	mustEqualString(t, "Diff(two hunks)", assert.Diff(before, after), "-1\n+one\n 2\n 3\n 4\n…\n 7\n 8\n 9\n-10\n+ten\n")
}

func (s *FormatTestSuite) Test_FormatMessage_NamedStringFormat(t *gotest.T, _ *ring0Ctx) {
	type format string
	mustEqualString(t, "FormatMessage([named format, args...])", assert.FormatMessage([]any{format("id %d"), 3}), "id 3")
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

type Node struct {
	Name string
	Next *Node
}

func (s *FormatTestSuite) TestRender_Cycle_IsNamedNotFollowed(t *gotest.T, _ *ring0Ctx) {
	n := &Node{Name: "a"}
	n.Next = n
	mustEqualString(t, "Render(cycle)", assert.Render(n), joinLines(
		"&assert_test.Node{",
		`  Name: "a",`,
		"  Next: (*assert_test.Node)(<cycle>),",
		"}",
	))
}

func (s *FormatTestSuite) TestRender_SharedPointer_IsRenderedEachTime(t *gotest.T, _ *ring0Ctx) {
	o := &Owner{"o"}
	mustEqualString(t, "Render(shared)", assert.Render([]*Owner{o, o}), joinLines(
		"[]*assert_test.Owner{",
		"  &assert_test.Owner{",
		`    Name: "o",`,
		"  },",
		"  &assert_test.Owner{",
		`    Name: "o",`,
		"  },",
		"}",
	))
}

type Kinds struct {
	Nil    []int
	Empty  []int
	M      map[string]int
	NilMap map[string]int
	F      func()
	NilF   func()
	C      chan int
	P      *int
	Any    any
	Err    error
	S      struct{}
	Arr    [2]bool
	U      uint8
	Ptr    uintptr
	Fl     float64
	Body   string
	When   time.Time
	secret *Point
}

func (s *FormatTestSuite) TestRender_EveryKind_OneEntryPerLineWithoutAddresses(t *gotest.T, _ *ring0Ctx) {
	five := 5
	v := Kinds{
		Empty: []int{}, M: map[string]int{"b": 2, "a": 1}, F: func() {}, C: make(chan int), P: &five,
		Any: int64(1), Arr: [2]bool{true, false}, U: 3, Ptr: 0x1f, Fl: 1.5, Body: "x\ny",
		When: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), secret: &Point{1, 2},
	}
	mustEqualString(t, "Render(kinds)", assert.Render(v), joinLines(
		"assert_test.Kinds{",
		"  Nil: []int(nil),",
		"  Empty: []int{},",
		"  M: map[string]int{",
		`    "a": 1,`,
		`    "b": 2,`,
		"  },",
		"  NilMap: map[string]int(nil),",
		"  F: (func())(<non-nil>),",
		"  NilF: (func())(nil),",
		"  C: (chan int)(<non-nil>),",
		"  P: &int(5),",
		"  Any: int64(1),",
		"  Err: error(nil),",
		"  S: struct {}{},",
		"  Arr: [2]bool{",
		"    true,",
		"    false,",
		"  },",
		"  U: 3,",
		"  Ptr: 0x1f,",
		"  Fl: 1.5,",
		`  Body: "x\n" +`,
		`    "y",`,
		"  When: time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC),",
		"  secret: &assert_test.Point{",
		"    X: 1,",
		"    Y: 2,",
		"  },",
		"}",
	))
}

func (s *FormatTestSuite) TestRender_MultilineGoString_KeepsItsLines(t *gotest.T, _ *ring0Ctx) {
	v := Box{V: multilineValue{[]string{"one", "two"}}}
	mustEqualString(t, "Render(GoString)", assert.Render(v), joinLines(
		"assert_test.Box{",
		"  V: one",
		"  two,",
		"}",
	))
}

func (s *FormatTestSuite) TestRender_HugeValue_EndsInATailSayingItWasCut(t *gotest.T, _ *ring0Ctx) {
	got := strings.Split(assert.Render(make([]int, 100000)), "\n")
	if len(got) != 50001 {
		fatalf(t, "Render(100000 ints) has %d lines; want 50000 and a tail", len(got))
	}
	mustEqualString(t, "line 49999", got[49999], "  0,")
	mustEqualString(t, "tail", got[50000], "… cut at 50000 lines")
}

// chain is n nodes, each the Next of the one before.
func chain(n int) *Node {
	var head *Node
	for range n {
		head = &Node{Name: "a", Next: head}
	}
	return head
}

func (s *FormatTestSuite) TestRender_DeepNesting_KeepsTheIndentOfDepth32AndMarksTheLevel(t *gotest.T, _ *ring0Ctx) {
	got := strings.Split(assert.Render(chain(34)), "\n")
	in := strings.Repeat(" ", 64)
	want := []string{
		in + "Next: &assert_test.Node{",
		in + `‹depth 33› Name: "a",`,
		in + "Next: &assert_test.Node{",
		in + `‹depth 34› Name: "a",`,
		in + "Next: (*assert_test.Node)(nil),",
		in + "‹depth 33› },",
		in + "},",
		strings.Repeat(" ", 62) + "},",
	}
	if len(got) < 72 {
		fatalf(t, "Render(34 nodes) has %d lines", len(got))
	}
	mustEqualString(t, "lines 64-71", strings.Join(got[64:72], "\n"), strings.Join(want, "\n"))
}

func (s *FormatTestSuite) TestRender_LongLines_AreCutAt4MiB(t *gotest.T, _ *ring0Ctx) {
	v := make([]string, 100)
	for i := range v {
		v[i] = strings.Repeat("x", 100000)
	}
	got := strings.Split(assert.Render(v), "\n")
	if len(got) != 43 {
		fatalf(t, "Render(100 strings of 100000 bytes) has %d lines; want 42 and a tail", len(got))
	}
	mustEqualString(t, "tail", got[42], "… cut at 4 MiB")
}

func (s *FormatTestSuite) TestRender_SyncValues_AreOpaque(t *gotest.T, _ *ring0Ctx) {
	var mu sync.RWMutex
	mustEqualString(t, "Render(sync)", assert.Render(struct {
		Mu *sync.RWMutex
		N  int
	}{&mu, 1}), joinLines(
		"struct { Mu *sync.RWMutex; N int }{",
		"  Mu: &sync.RWMutex{…},",
		"  N: 1,",
		"}",
	))
}
