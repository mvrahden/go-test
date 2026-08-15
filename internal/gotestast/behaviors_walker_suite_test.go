package gotestast_test

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BehaviorWalkerTestSuite reads testdata/behaviors and pins what the walker
// says about it. The names asserted here are the names go test produces for
// that same source — the walker exists to predict them, so anything it gets
// wrong shows up as a behavior that can never be addressed by the name it was
// given.
type BehaviorWalkerTestSuite struct {
	specs map[string]gotestast.MethodSpec
}

func (s *BehaviorWalkerTestSuite) BeforeAll(t *gotest.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "behaviors"))
	gotest.NoError(t, err)

	loaded, broken, err := gotestgen.LoadPackagesForDiscovery([]string{dir}, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken, "the walker's own source must compile")
	gotest.NotEmpty(t, loaded)

	s.specs = map[string]gotestast.MethodSpec{}
	collector := gotestgen.NewCollector()
	for _, lr := range loaded {
		if lr.Ptest == nil {
			continue
		}
		result := collector.CollectSuiteSpecs(lr.Ptest)
		gotest.Empty(t, result.Errs)
		for _, suite := range result.Suites {
			for _, method := range suite.TestCases() {
				s.specs[method.Identifier()] = suite.BehaviorsOf(method)
			}
		}
	}
	gotest.NotEmpty(t, s.specs, "no suites were collected from testdata")
}

func (s *BehaviorWalkerTestSuite) AfterAll(t *gotest.T) {
	s.specs = nil
}

// paths renders a method's tree as one line per node, so an assertion states
// the whole shape rather than probing it field by field.
func paths(spec gotestast.MethodSpec) []string {
	var out []string
	var walk func(prefix string, behaviors []*gotestast.Behavior)
	walk = func(prefix string, behaviors []*gotestast.Behavior) {
		for _, b := range behaviors {
			path := prefix + b.Name
			out = append(out, path)
			walk(path+"/", b.Children)
		}
	}
	walk("", spec.Behaviors)
	return out
}

func (s *BehaviorWalkerTestSuite) TestTreeShape(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc   string
		method string
		want   []string
	}{
		{
			Desc:   "nesting is as deep as it is written",
			method: "TestNesting",
			want: []string{
				"a_group",
				"a_group/has_a_leaf",
				"a_group/a_deeper_group",
				"a_group/a_deeper_group/has_its_own_leaf",
			},
		},
		{
			// go test appends #01 to the second sibling of a name, #02 to the
			// third. Without it two behaviors collapse onto one name.
			Desc:   "repeated siblings are numbered the way go test numbers them",
			method: "TestDuplicateSiblings",
			want: []string{
				"same_name",
				"same_name/first",
				"same_name#01",
				"same_name#01/second",
				"same_name#02",
				"same_name#02/third",
			},
		},
		{
			// A single slash is a level separator, so "a/b" and "a/c" share one
			// parent; a run of slashes is not, so "https://" stays one level.
			Desc:   "a slash in a description is a level, a run of them is not",
			method: "TestSlashes",
			want: []string{
				"a",
				"a/b_grouping",
				"a/b_grouping/works",
				"a/c_grouping",
				"a/c_grouping/also_works",
				"https://_URI",
				"https://_URI/stays_one_level",
			},
		},
		{
			Desc:   "a keyed table names its rows from Desc",
			method: "TestKeyedTable",
			want: []string{
				"negative", "negative/classifies",
				"zero", "zero/classifies",
			},
		},
		{
			Desc:   "a positional table names its rows from the same field",
			method: "TestPositionalTable",
			want: []string{
				"too_short", "too_short/checks_the_length",
				"long_enough", "long_enough/checks_the_length",
			},
		},
		{
			Desc:   "a table with nothing to name its rows falls back to the index",
			method: "TestUnnamedTable",
			want: []string{
				"#0", "#0/still_runs",
				"#1", "#1/still_runs",
			},
		},
		{
			Desc:   "only the unconditional behavior is declared",
			method: "TestConditional",
			want:   []string{"is_always_declared"},
		},
		{
			Desc:   "a description that is not a literal declares nothing",
			method: "TestComputedDescription",
			want:   nil,
		},
		{
			Desc:   "a table that is not a literal declares no rows",
			method: "TestNonLiteralTable",
			want:   nil,
		},
		{
			Desc:   "a method with no behaviors has an empty tree",
			method: "TestNoBehaviorsAtAll",
			want:   nil,
		},
	}) {
		spec, ok := s.specs[tc.method]
		gotest.True(sub, ok, "method %s was not collected", tc.method)
		gotest.Equal(sub, tc.want, paths(spec))
	}
}

func (s *BehaviorWalkerTestSuite) TestCompleteness(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		method   string
		complete bool
		note     string
	}{
		{Desc: "a tree of literals is exhaustive", method: "TestNesting", complete: true},
		{Desc: "a literal table is exhaustive", method: "TestKeyedTable", complete: true},
		{Desc: "a method with no behaviors is exhaustive", method: "TestNoBehaviorsAtAll", complete: true},
		{
			Desc: "a behavior behind a condition is not", method: "TestConditional",
			complete: false, note: "depend on runtime values",
		},
		{
			Desc: "a computed description is not", method: "TestComputedDescription",
			complete: false, note: "not a string literal",
		},
		{
			Desc: "a table that is not a literal is not", method: "TestNonLiteralTable",
			complete: false, note: "not a literal table",
		},
	}) {
		spec, ok := s.specs[tc.method]
		gotest.True(sub, ok, "method %s was not collected", tc.method)
		gotest.Equal(sub, tc.complete, spec.Complete)
		if tc.complete {
			gotest.Empty(sub, spec.Notes)
			continue
		}
		// The note has to name the construct that defeated the walker, or the
		// developer has no way to act on it.
		gotest.NotEmpty(sub, spec.Notes)
		gotest.True(sub, containsNote(spec.Notes, tc.note),
			"no note mentioning %q in %v", tc.note, spec.Notes)
	}
}

func containsNote(notes []string, want string) bool {
	for _, n := range notes {
		if strings.Contains(n, want) {
			return true
		}
	}
	return false
}

// Every row of a table gets its own copy of the behaviors written in the loop
// body. Sharing the nodes would make one row's subtree the other's.
func (s *BehaviorWalkerTestSuite) TestTableRowsDoNotShareNodes(t *gotest.T) {
	t.When("a table declares behaviors in its loop body", func(w *gotest.T) {
		spec := s.specs["TestKeyedTable"]
		gotest.Len(w, spec.Behaviors, 2)

		w.It("gives each row its own subtree", func(it *gotest.T) {
			first := spec.Behaviors[0].Children[0]
			second := spec.Behaviors[1].Children[0]
			gotest.Equal(it, first.Name, second.Name)
			// Same name, and it has to be a different node: the rows are
			// separate subtests, and one aliasing the other would give a
			// verdict on one row to both.
			gotest.NotEqual(it,
				reflect.ValueOf(first).Pointer(),
				reflect.ValueOf(second).Pointer(),
				"the two rows point at one node")
		})

		w.It("records where each row was written", func(it *gotest.T) {
			lines := []int{spec.Behaviors[0].Line, spec.Behaviors[1].Line}
			sort.Ints(lines)
			gotest.Less(it, lines[0], lines[1], "rows must point at their own source line")
		})
	})
}

// Display is what the developer wrote; Name is what go test will report. The
// two differ, and the tree needs both — one to read, one to address.
func (s *BehaviorWalkerTestSuite) TestDisplayKeepsTheProse(t *gotest.T) {
	t.When("a description contains spaces", func(w *gotest.T) {
		spec := s.specs["TestNesting"]

		w.It("keeps the prose in Display", func(it *gotest.T) {
			gotest.Equal(it, "a group", spec.Behaviors[0].Display)
		})

		w.It("rewrites only the Name", func(it *gotest.T) {
			gotest.Equal(it, "a_group", spec.Behaviors[0].Name)
		})
	})

	t.When("a description is split across levels by a slash", func(w *gotest.T) {
		spec := s.specs["TestSlashes"]

		w.It("gives each level the piece of prose it came from", func(it *gotest.T) {
			gotest.Equal(it, "a", spec.Behaviors[0].Display)
			gotest.Equal(it, "b grouping", spec.Behaviors[0].Children[0].Display)
		})
	})
}
