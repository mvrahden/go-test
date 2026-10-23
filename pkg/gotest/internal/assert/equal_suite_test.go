// Ring 0: raw checks only (see ring0_suite_test.go).
package assert_test //nolint:fail-guard

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

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
	mustEqualMessage(t, "unequal multiline values", assert.CheckEqual(expected, actual), joinLines(
		"Equal failed:",
		"  expected: line one",
		"            line two",
		"            line three",
		"  actual:   line one",
		"            line changed",
		"            line three",
		"  diff (-expected +actual):",
		"     line one",
		"    -line two",
		"    +line changed",
		"     line three",
	))
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

func (s *EqualTestSuite) TestCheckEqual_UnexportedOnlyDifference_IsDiffed(t *gotest.T, _ *ring0Ctx) {
	a := partlyHidden{Exported: "long-enough-value-to-cross-the-threshold", Padding: "x", secret: 1}
	b := partlyHidden{Exported: "long-enough-value-to-cross-the-threshold", Padding: "x", secret: 2}
	mustEqualMessage(t, "unexported-only difference", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  expected: assert_test.partlyHidden{",
		`              Exported: "long-enough-value-to-cross-the-threshold",`,
		`              Padding: "x",`,
		"              secret: 1,",
		"            }",
		"  actual:   assert_test.partlyHidden{",
		`              Exported: "long-enough-value-to-cross-the-threshold",`,
		`              Padding: "x",`,
		"              secret: 2,",
		"            }",
		"  diff (-expected +actual):",
		"    at .secret:",
		"     assert_test.partlyHidden{",
		`       Exported: "long-enough-value-to-cross-the-threshold",`,
		`       Padding: "x",`,
		"    -  secret: 1,",
		"    +  secret: 2,",
		"     }",
	))
}

// joinLines is a failure message spelled one line per argument.
func joinLines(lines ...string) string { return strings.Join(lines, "\n") }

func mustEqualMessage(t *gotest.T, what, got, want string) {
	if got != want {
		fatalf(t, "%s: message\n%s\nwant\n%s", what, got, want)
	}
}

type Point struct{ X, Y int }

type Address struct{ Street, City, Zip string }

type User struct {
	ID      int
	Name    string
	Address Address
	Tags    []string
	Manager *User
	secret  string
}

func newUser() User {
	return User{
		ID: 7, Name: "Ada Lovelace",
		Address: Address{Street: "12 St James's Square", City: "London", Zip: "SW1Y 4JH"},
		Tags:    []string{"admin", "math"},
		Manager: &User{ID: 1, Name: "Babbage"},
	}
}

func (s *EqualTestSuite) TestCheckEqual_ShortValues_ShowBothWithoutDiff(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "small structs", assert.CheckEqual(Point{1, 2}, Point{1, 3}), joinLines(
		"Equal failed:",
		"  expected: assert_test.Point{X:1, Y:2}",
		"  actual:   assert_test.Point{X:1, Y:3}",
	))
}

func (s *EqualTestSuite) TestCheckEqual_NilVsEmptySlice(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "nil vs empty slice", assert.CheckEqual([]int(nil), []int{}), joinLines(
		"Equal failed:",
		"  expected: []int(nil)",
		"  actual:   []int{}",
	))
}

type Box struct{ V any }

func (s *EqualTestSuite) TestCheckEqual_InterfaceFieldsOfDifferentDynamicTypes(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "int vs int64 in an interface", assert.CheckEqual(Box{V: 1}, Box{V: int64(1)}), joinLines(
		"Equal failed:",
		"  expected: assert_test.Box{V:int(1)}",
		"  actual:   assert_test.Box{V:int64(1)}",
	))
}

func (s *EqualTestSuite) TestCheckEqual_NestedFieldChange_MarksOnlyThatField(t *gotest.T, _ *ring0Ctx) {
	a, b := newUser(), newUser()
	b.Address.Zip = "SW1Y 4JJ"
	mustEqualMessage(t, "nested field", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at .Address.Zip:",
		"       Address: assert_test.Address{",
		`         Street: "12 St James's Square",`,
		`         City: "London",`,
		`    -    Zip: "SW1Y 4JH",`,
		`    +    Zip: "SW1Y 4JJ",`,
		"       },",
		"       Tags: []string{",
		`         "admin",`,
	))
}

func (s *EqualTestSuite) TestCheckEqual_ChangeBehindPointer_IsFollowed(t *gotest.T, _ *ring0Ctx) {
	a, b := newUser(), newUser()
	b.Manager.Name = "Charles Babbage"
	mustEqualMessage(t, "field behind a pointer", assert.CheckEqual(&a, &b), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at .Manager.Name:",
		"       },",
		"       Manager: &assert_test.User{",
		"         ID: 1,",
		`    -    Name: "Babbage",`,
		`    +    Name: "Charles Babbage",`,
		"         Address: assert_test.Address{",
		`           Street: "",`,
		`           City: "",`,
	))
}

func (s *EqualTestSuite) TestCheckEqual_UnexportedFieldChange_IsShown(t *gotest.T, _ *ring0Ctx) {
	a, b := newUser(), newUser()
	b.secret = "x"
	mustEqualMessage(t, "unexported field", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at .secret:",
		"         Manager: (*assert_test.User)(nil),",
		`         secret: "",`,
		"       },",
		`    -  secret: "",`,
		`    +  secret: "x",`,
		"     }",
	))
}

type Owner struct{ Name string }

type Item struct {
	Name  string
	Owner *Owner
}

func (s *EqualTestSuite) TestCheckEqual_SliceOfStructs_MarksOnlyTheChangedElement(t *gotest.T, _ *ring0Ctx) {
	a := []Item{{"a", &Owner{"o"}}, {"b", &Owner{"o"}}}
	b := []Item{{"a", &Owner{"o"}}, {"B", &Owner{"o"}}}
	mustEqualMessage(t, "slice of structs", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at [1].Name:",
		"         },",
		"       },",
		"       assert_test.Item{",
		`    -    Name: "b",`,
		`    +    Name: "B",`,
		"         Owner: &assert_test.Owner{",
		`           Name: "o",`,
		"         },",
	))
}

func sequence(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func (s *EqualTestSuite) TestCheckEqual_LongSlice_DiffsPastAnyEntryCap(t *gotest.T, _ *ring0Ctx) {
	a, b := make([]int, 100), make([]int, 100)
	b[57] = 1
	mustEqualMessage(t, "100 ints", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at [57]:",
		"       0,",
		"       0,",
		"       0,",
		"    -  0,",
		"    +  1,",
		"       0,",
		"       0,",
		"       0,",
	))
}

func (s *EqualTestSuite) TestCheckEqual_SliceInsertion_IsOneAddedLine(t *gotest.T, _ *ring0Ctx) {
	a := sequence(100)
	b := append(append(append([]int(nil), a[:50]...), 1000), a[50:]...)
	mustEqualMessage(t, "insertion", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at [50]:",
		"       47,",
		"       48,",
		"       49,",
		"    +  1000,",
		"       50,",
		"       51,",
		"       52,",
	))
}

func (s *EqualTestSuite) TestCheckEqual_DistantChanges_AreSeparateHunks(t *gotest.T, _ *ring0Ctx) {
	a, b := sequence(100), sequence(100)
	b[10], b[80] = -1, -2
	mustEqualMessage(t, "two hunks", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at [10]:",
		"       7,",
		"       8,",
		"       9,",
		"    -  10,",
		"    +  -1,",
		"       11,",
		"       12,",
		"       13,",
		"    …",
		"    at [80]:",
		"       77,",
		"       78,",
		"       79,",
		"    -  80,",
		"    +  -2,",
		"       81,",
		"       82,",
		"       83,",
	))
}

func (s *EqualTestSuite) TestCheckEqual_SmallMultilineValues_ShowBlocksAndDiff(t *gotest.T, _ *ring0Ctx) {
	a := map[string]string{"echo": "five", "alpha": "one", "delta": "four", "bravo": "two", "charlie": "three"}
	b := map[string]string{"charlie": "three", "bravo": "two", "echo": "five", "alpha": "one", "delta": "FOUR"}
	mustEqualMessage(t, "map", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  expected: map[string]string{",
		`              "alpha": "one",`,
		`              "bravo": "two",`,
		`              "charlie": "three",`,
		`              "delta": "four",`,
		`              "echo": "five",`,
		"            }",
		"  actual:   map[string]string{",
		`              "alpha": "one",`,
		`              "bravo": "two",`,
		`              "charlie": "three",`,
		`              "delta": "FOUR",`,
		`              "echo": "five",`,
		"            }",
		"  diff (-expected +actual):",
		`    at ["delta"]:`,
		`       "alpha": "one",`,
		`       "bravo": "two",`,
		`       "charlie": "three",`,
		`    -  "delta": "four",`,
		`    +  "delta": "FOUR",`,
		`       "echo": "five",`,
		"     }",
	))
}

func (s *EqualTestSuite) TestCheckEqual_MultilineString_IsDiffedByLine(t *gotest.T, _ *ring0Ctx) {
	got := assert.CheckEqual("line one\nline two\nline three\n", "line one\nline 2\nline three\n")
	mustEqualMessage(t, "multi-line string", got, joinLines(
		"Equal failed:",
		`  expected: "line one\n" +`,
		`              "line two\n" +`,
		`              "line three\n"`,
		`  actual:   "line one\n" +`,
		`              "line 2\n" +`,
		`              "line three\n"`,
		"  diff (-expected +actual):",
		`     "line one\n" +`,
		`    -  "line two\n" +`,
		`    +  "line 2\n" +`,
		`       "line three\n"`,
	))
}

func (s *EqualTestSuite) TestCheckEqual_LongSingleLineString_PointsAtTheFirstDifference(t *gotest.T, _ *ring0Ctx) {
	a := strings.Repeat("a", 50) + "X" + strings.Repeat("b", 50)
	b := strings.Repeat("a", 50) + "Y" + strings.Repeat("b", 50)
	mustEqualMessage(t, "long string", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  strings differ at byte 50 (expected 101 bytes, actual 101)",
		`  expected: …"`+strings.Repeat("a", 30)+"X"+strings.Repeat("b", 29)+`"…`,
		`  actual:   …"`+strings.Repeat("a", 30)+"Y"+strings.Repeat("b", 29)+`"…`,
		strings.Repeat(" ", 44)+"^",
	))
}

func (s *EqualTestSuite) TestCheckEqual_LongStringPrefix_PointsAtItsEnd(t *gotest.T, _ *ring0Ctx) {
	a := strings.Repeat("a", 90)
	mustEqualMessage(t, "prefix", assert.CheckEqual(a, a+"z"), joinLines(
		"Equal failed:",
		"  strings differ at byte 90 (expected 90 bytes, actual 91)",
		`  expected: …"`+strings.Repeat("a", 30)+`"`,
		`  actual:   …"`+strings.Repeat("a", 30)+`z"`,
		strings.Repeat(" ", 44)+"^",
	))
}

func (s *EqualTestSuite) TestCheckEqual_NaN_SaysTheRenderingHidesTheDifference(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "NaN", assert.CheckEqual(math.NaN(), math.NaN()), joinLines(
		"Equal failed:",
		"  values differ in ways their rendering does not show (NaN, a func, or state the rendering omits)",
		"  both:     NaN",
	))
}

func (s *EqualTestSuite) TestCheckEqual_MonotonicClockOnly_DiffsTheRawFields(t *gotest.T, _ *ring0Ctx) {
	now := time.Now()
	got := assert.CheckEqual(now, now.Round(0))
	mustFail(t, got, "monotonic clock", "Equal failed:\n  the values render alike; their raw fields differ:\n", "\n    at .wall:\n", "\n    -  wall: ", "\n    +  wall: ", "\n    -  ext: ", "\n    +  ext: ")
	mustNotContain(t, "monotonic clock", got, now.Round(0).GoString())
}

func (s *EqualTestSuite) TestCheckEqual_DifferenceBeyondTheLineCap_IsNamed(t *gotest.T, _ *ring0Ctx) {
	a, b := sequence(100000), sequence(100000)
	b[99999] = -1
	mustEqualMessage(t, "beyond the cap", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  the renderings are cut at 50000 lines; the values first differ at [99999]",
	))
}

func mustNotContain(t *gotest.T, what, got, fragment string) {
	if strings.Contains(got, fragment) {
		t.Errorf("%s: message should not contain %q, got: %q", what, fragment, got)
	}
}

type MyStr string

func (s *EqualTestSuite) TestCheckEqual_DifferentDynamicTypes_AreNamedAndTyped(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "int vs float64", assert.CheckEqual(any(1), any(float64(1))), joinLines(
		"Equal failed:",
		"  types differ: int vs float64",
		"  expected: int(1)",
		"  actual:   float64(1)",
	))
	mustEqualMessage(t, "string vs a named string", assert.CheckEqual("a", MyStr("a")), joinLines(
		"Equal failed:",
		"  types differ: string vs assert_test.MyStr",
		`  expected: string("a")`,
		`  actual:   assert_test.MyStr("a")`,
	))
	long := strings.Repeat("a", 90)
	mustEqualMessage(t, "long strings of two types", assert.CheckEqual(long, MyStr(long)), joinLines(
		"Equal failed:",
		"  types differ: string vs assert_test.MyStr",
		`  expected: string("`+long+`")`,
		`  actual:   assert_test.MyStr("`+long+`")`,
	))
}

type withPointer struct {
	P *int
	F float64
}

func (s *EqualTestSuite) TestCheckEqual_NaNBehindDistinctPointers_NamesWhereWithoutAddresses(t *gotest.T, _ *ring0Ctx) {
	x, y := 1, 1
	mustEqualMessage(t, "NaN beside a pointer", assert.CheckEqual(withPointer{&x, math.NaN()}, withPointer{&y, math.NaN()}), joinLines(
		"Equal failed:",
		"  values differ at .F in ways their rendering does not show (NaN, a func, or state the rendering omits)",
		"  both:     assert_test.withPointer{P:&int(1), F:NaN}",
	))
}

func (s *EqualTestSuite) TestCheckEqual_Funcs_NamedWithoutAddresses(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "funcs", assert.CheckEqual([]func(){func() {}}, []func(){func() {}}), joinLines(
		"Equal failed:",
		"  values differ at [0] in ways their rendering does not show (NaN, a func, or state the rendering omits)",
		"  both:     []func(){(func())(<non-nil>)}",
	))
}

// masked's GoString leaves out a field, so two values can render alike.
type masked struct{ Shown, hidden int }

func (m masked) GoString() string { return "masked(" + strconv.Itoa(m.Shown) + ")" }

func (s *EqualTestSuite) TestCheckEqual_GoStringsAlike_ShowsTheRawFields(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "GoStrings alike", assert.CheckEqual(masked{1, 2}, masked{1, 3}), joinLines(
		"Equal failed:",
		"  the values render alike; their raw fields differ:",
		"  expected: assert_test.masked{Shown:1, hidden:2}",
		"  actual:   assert_test.masked{Shown:1, hidden:3}",
	))
}

func (s *EqualTestSuite) TestCheckEqual_KeysThatRenderAlike_AreOrderedByValue(t *gotest.T, _ *ring0Ctx) {
	p1, p2, p3 := &Point{1, 1}, &Point{1, 1}, &Point{1, 1}
	e := map[*Point]int{p1: 1, p2: 2, p3: 3}
	a := map[*Point]int{p1: 1, p2: 2, p3: 4}
	want := joinLines(
		"Equal failed:",
		"  expected: map[*assert_test.Point]int{",
		"              &assert_test.Point{X:1, Y:1}: 1,",
		"              &assert_test.Point{X:1, Y:1}: 2,",
		"              &assert_test.Point{X:1, Y:1}: 3,",
		"            }",
		"  actual:   map[*assert_test.Point]int{",
		"              &assert_test.Point{X:1, Y:1}: 1,",
		"              &assert_test.Point{X:1, Y:1}: 2,",
		"              &assert_test.Point{X:1, Y:1}: 4,",
		"            }",
		"  diff (-expected +actual):",
		"    at [&assert_test.Point{X:1, Y:1}]:",
		"     map[*assert_test.Point]int{",
		"       &assert_test.Point{X:1, Y:1}: 1,",
		"       &assert_test.Point{X:1, Y:1}: 2,",
		"    -  &assert_test.Point{X:1, Y:1}: 3,",
		"    +  &assert_test.Point{X:1, Y:1}: 4,",
		"     }",
	)
	for range 50 {
		mustEqualMessage(t, "pointer keys", assert.CheckEqual(e, a), want)
	}
}

func (s *EqualTestSuite) TestCheckEqual_NaNKeys_AreOrderedByValue(t *gotest.T, _ *ring0Ctx) {
	nan := math.NaN()
	e := map[float64]string{1: "first entry", 2: "second entry"}
	e[nan], e[nan] = "nan one", "nan two"
	a := map[float64]string{1: "first entry", 2: "SECOND entry"}
	a[nan], a[nan] = "nan two", "nan one"
	want := joinLines(
		"Equal failed:",
		"  expected: map[float64]string{",
		`              1: "first entry",`,
		`              2: "second entry",`,
		`              NaN: "nan one",`,
		`              NaN: "nan two",`,
		"            }",
		"  actual:   map[float64]string{",
		`              1: "first entry",`,
		`              2: "SECOND entry",`,
		`              NaN: "nan one",`,
		`              NaN: "nan two",`,
		"            }",
		"  diff (-expected +actual):",
		"    at [2]:",
		"     map[float64]string{",
		`       1: "first entry",`,
		`    -  2: "second entry",`,
		`    +  2: "SECOND entry",`,
		`       NaN: "nan one",`,
		`       NaN: "nan two",`,
		"     }",
	)
	for range 50 {
		mustEqualMessage(t, "NaN keys", assert.CheckEqual(e, a), want)
	}
}

func (s *EqualTestSuite) TestCheckEqual_TextBytes_AreDiffedByLine(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "text bytes", assert.CheckEqual([]byte("line one\nline two\n"), []byte("line one\nline 2\n")), joinLines(
		"Equal failed:",
		`  expected: []byte("line one\n" +`,
		`              "line two\n")`,
		`  actual:   []byte("line one\n" +`,
		`              "line 2\n")`,
		"  diff (-expected +actual):",
		`     []byte("line one\n" +`,
		`    -  "line two\n")`,
		`    +  "line 2\n")`,
	))
}

// payload is a named byte slice, as json.RawMessage is.
type payload []byte

type document struct {
	Body payload
	Rev  int
}

func (s *EqualTestSuite) TestCheckEqual_ShortJSONBytes_ShowAsText(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "JSON bytes", assert.CheckEqual(document{payload(`{"age":36}`), 1}, document{payload(`{"age":37}`), 1}), joinLines(
		"Equal failed:",
		`  expected: assert_test.document{Body:assert_test.payload("{\"age\":36}"), Rev:1}`,
		`  actual:   assert_test.document{Body:assert_test.payload("{\"age\":37}"), Rev:1}`,
	))
}

func (s *EqualTestSuite) TestCheckEqual_LongSingleLineBytes_PointAtTheFirstDifference(t *gotest.T, _ *ring0Ctx) {
	a := []byte(strings.Repeat("a", 50) + "X" + strings.Repeat("b", 50))
	b := []byte(strings.Repeat("a", 50) + "Y" + strings.Repeat("b", 50))
	mustEqualMessage(t, "long bytes", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  byte slices differ at byte 50 (expected 101 bytes, actual 101)",
		`  expected: …"`+strings.Repeat("a", 30)+"X"+strings.Repeat("b", 29)+`"…`,
		`  actual:   …"`+strings.Repeat("a", 30)+"Y"+strings.Repeat("b", 29)+`"…`,
		strings.Repeat(" ", 44)+"^",
	))
}

func (s *EqualTestSuite) TestCheckEqual_BinaryBytes_RenderAsHexRows(t *gotest.T, _ *ring0Ctx) {
	a := []byte{0xff, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}
	b := append([]byte(nil), a...)
	b[12] = 0xee
	mustEqualMessage(t, "binary bytes", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  expected: []byte{",
		"              0xff, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,",
		"              0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,",
		"              0x10, 0x11,",
		"            }",
		"  actual:   []byte{",
		"              0xff, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,",
		"              0x08, 0x09, 0x0a, 0x0b, 0xee, 0x0d, 0x0e, 0x0f,",
		"              0x10, 0x11,",
		"            }",
		"  diff (-expected +actual):",
		"    at [12]:",
		"     []byte{",
		"       0xff, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,",
		"    -  0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,",
		"    +  0x08, 0x09, 0x0a, 0x0b, 0xee, 0x0d, 0x0e, 0x0f,",
		"       0x10, 0x11,",
		"     }",
	))
}

func (s *EqualTestSuite) TestCheckEqual_CutRenderings_NameTheFirstDifferingKey(t *gotest.T, _ *ring0Ctx) {
	a, b := map[string]int{}, map[string]int{}
	for i := range 100000 {
		a["k"+strconv.Itoa(i)], b["k"+strconv.Itoa(i)] = i, i
	}
	b["k9"] = -1
	mustEqualMessage(t, "cut map", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		`  the renderings are cut at 50000 lines; the values first differ at ["k9"]`,
	))
}

func (s *EqualTestSuite) TestCheckEqual_DeletedElement_IsMarkedWhole(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "deleted element", assert.CheckEqual([]Point{{1, 0}, {2, 0}, {3, 0}, {4, 0}}, []Point{{1, 0}, {3, 0}, {4, 0}}), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at [1]:",
		"         X: 1,",
		"         Y: 0,",
		"       },",
		"    -  assert_test.Point{",
		"    -    X: 2,",
		"    -    Y: 0,",
		"    -  },",
		"       assert_test.Point{",
		"         X: 3,",
		"         Y: 0,",
	))
}

type guarded struct {
	sync.Mutex
	N int
}

func (s *EqualTestSuite) TestCheckEqual_SyncValues_AreOpaque(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "sync values", assert.CheckEqual(&guarded{N: 1}, &guarded{N: 2}), joinLines(
		"Equal failed:",
		"  expected: &assert_test.guarded{Mutex:sync.Mutex{…}, N:1}",
		"  actual:   &assert_test.guarded{Mutex:sync.Mutex{…}, N:2}",
	))
}

func (s *EqualTestSuite) TestCheckEqual_SyncStateOnly_ShowsTheRawFields(t *gotest.T, _ *ring0Ctx) {
	locked := &guarded{}
	locked.Lock()
	got := assert.CheckEqual(&guarded{}, locked)
	mustFail(t, got, "a locked mutex", "Equal failed:\n  the values render alike; their raw fields differ:\n")
	mustNotContain(t, "a locked mutex", got, "{…}")
}

type twoTexts struct{ A, B string }

func (s *EqualTestSuite) TestCheckEqual_LongChangedLine_PointsAtTheDifference(t *gotest.T, _ *ring0Ctx) {
	long := strings.Repeat("x", 300)
	mustEqualMessage(t, "long changed line", assert.CheckEqual(twoTexts{long + "1", "q"}, twoTexts{long + "2", "q"}), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"    at .A:",
		"     assert_test.twoTexts{",
		`    -  …`+strings.Repeat("x", 30)+`1",`,
		`    +  …`+strings.Repeat("x", 30)+`2",`,
		strings.Repeat(" ", 38)+"^",
		`       B: "q",`,
		"     }",
	))
}

func (s *EqualTestSuite) TestCheckEqual_WideRunes_CaretCountsTheirWidth(t *gotest.T, _ *ring0Ctx) {
	a := strings.Repeat("日", 30) + "X" + strings.Repeat("b", 10)
	b := strings.Repeat("日", 30) + "Y" + strings.Repeat("b", 10)
	mustEqualMessage(t, "wide runes", assert.CheckEqual(a, b), joinLines(
		"Equal failed:",
		"  strings differ at byte 90 (expected 101 bytes, actual 101)",
		`  expected: …"`+strings.Repeat("日", 10)+"X"+strings.Repeat("b", 10)+`"`,
		`  actual:   …"`+strings.Repeat("日", 10)+"Y"+strings.Repeat("b", 10)+`"`,
		strings.Repeat(" ", 34)+"^",
	))
}

func (s *EqualTestSuite) TestCheckEqual_HugeKeys_EveryLineStaysBounded(t *gotest.T, _ *ring0Ctx) {
	a, b := map[string]int{}, map[string]int{}
	for i := range 3 {
		k := strings.Repeat(strconv.Itoa(i), 1<<20)
		a[k], b[k] = i, i
	}
	b[strings.Repeat("1", 1<<20)] = -1
	got := assert.CheckEqual(a, b)
	mustFail(t, got, "huge keys", "\n    at [\"1111", "\n    -  …1111", "\n    +  …1111")
	for _, l := range strings.Split(got, "\n") {
		if len(l) > 200 {
			fatalf(t, "huge keys: a line of %d bytes: %.100q…", len(l), l)
		}
	}
}

func (s *EqualTestSuite) TestCheckNotEqual_ShowsTheValueWithoutAddresses(t *gotest.T, _ *ring0Ctx) {
	v := Item{"a", &Owner{"o"}}
	mustEqualMessage(t, "NotEqual", assert.CheckNotEqual(v, v), joinLines(
		"NotEqual failed:",
		`  both are: assert_test.Item{Name:"a", Owner:&assert_test.Owner{Name:"o"}}`,
	))
}

func (s *EqualTestSuite) TestCheckEqual_FinalNewlineOnOneSide_IsMarked(t *gotest.T, _ *ring0Ctx) {
	mustEqualMessage(t, "final newline", assert.CheckEqual(multilineValue{[]string{"a", "b", ""}}, multilineValue{[]string{"a", "b"}}), joinLines(
		"Equal failed:",
		"  expected: a",
		"            b",
		"  actual:   a",
		"            b",
		"  diff (-expected +actual):",
		"     a",
		"    -b",
		"    +b",
		`    \ No newline at end of file`,
	))
}

func (s *EqualTestSuite) TestCheckEqual_LongLastLineWithoutNewline_IsWindowedAndMarked(t *gotest.T, _ *ring0Ctx) {
	long := strings.Repeat("x", 100)
	window := "…" + strings.Repeat("x", 30)
	mustEqualMessage(t, "expected unended", assert.CheckEqual(multilineValue{[]string{"a", long + "1"}}, multilineValue{[]string{"a", long + "2", ""}}), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"     a",
		"    -"+window+"1",
		`    \ No newline at end of file`,
		"    +"+window+"2",
		strings.Repeat(" ", 36)+"^",
	))
	mustEqualMessage(t, "actual unended", assert.CheckEqual(multilineValue{[]string{"a", long + "1", ""}}, multilineValue{[]string{"a", long + "2"}}), joinLines(
		"Equal failed:",
		"  diff (-expected +actual):",
		"     a",
		"    -"+window+"1",
		"    +"+window+"2",
		`    \ No newline at end of file`,
		strings.Repeat(" ", 36)+"^",
	))
}

func (s *EqualTestSuite) TestCheckEqual_LongSingleLineRenderings_PointAtTheFirstDifference(t *gotest.T, _ *ring0Ctx) {
	x, y := strings.Repeat("y", 200)+"1", strings.Repeat("y", 200)+"2"
	mustEqualMessage(t, "pointers to long strings", assert.CheckEqual(&x, &y), joinLines(
		"Equal failed:",
		`  expected: …`+strings.Repeat("y", 30)+`1")`,
		`  actual:   …`+strings.Repeat("y", 30)+`2")`,
		strings.Repeat(" ", 43)+"^",
	))
}

// label is a named string kind.
type label string

func (s *EqualTestSuite) TestCheckEqual_LongStringsOfDifferentTypes_PointAtTheFirstDifference(t *gotest.T, _ *ring0Ctx) {
	l := strings.Repeat("y", 200)
	mustEqualMessage(t, "long strings of two types", assert.CheckEqual(any(l+"1"), any(label(l+"2"))), joinLines(
		"Equal failed:",
		"  types differ: string vs assert_test.label",
		"  strings differ at byte 200 (expected 201 bytes, actual 201)",
		`  expected: …"`+strings.Repeat("y", 30)+`1"`,
		`  actual:   …"`+strings.Repeat("y", 30)+`2"`,
		strings.Repeat(" ", 44)+"^",
	))
}

func (s *EqualTestSuite) TestCheckEqual_BinaryBytesOneLonger_NameTheFirstExtraByte(t *gotest.T, _ *ring0Ctx) {
	a := []byte{0xff, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	got := assert.CheckEqual(a, append(a[:17:17], 17))
	mustFail(t, got, "one byte longer", "\n    at [17]:\n     []byte{\n", "\n    -  0x10,\n    +  0x10, 0x11,\n")
}
