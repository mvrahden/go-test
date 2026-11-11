// Source the behavior walker is read against. It lives under testdata so the
// go tool never builds it as part of the repository, and it is written to be
// adverse: every construct here is one the walker either has to model exactly
// or has to admit it cannot see.
package behaviors

import (
	"github.com/mvrahden/go-test/internal/gotestast/testdata/behaviors/names"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type WalkerTestSuite struct{}

func (s *WalkerTestSuite) TestNesting(t *gotest.T) {
	t.When("a group", func(t *gotest.T) {
		t.It("has a leaf", func(t *gotest.T) { gotest.True(t, true) })
		t.When("a deeper group", func(t *gotest.T) {
			t.It("has its own leaf", func(t *gotest.T) { gotest.True(t, true) })
		})
	})
}

// Two siblings sharing a description: go test names the second one "#01".
func (s *WalkerTestSuite) TestDuplicateSiblings(t *gotest.T) {
	t.When("same name", func(t *gotest.T) {
		t.It("first", func(t *gotest.T) { gotest.True(t, true) })
	})
	t.When("same name", func(t *gotest.T) {
		t.It("second", func(t *gotest.T) { gotest.True(t, true) })
	})
	t.When("same name", func(t *gotest.T) {
		t.It("third", func(t *gotest.T) { gotest.True(t, true) })
	})
}

// A slash in a label is part of it: one subtest, however many slashes.
func (s *WalkerTestSuite) TestSlashes(t *gotest.T) {
	t.When("a/b grouping", func(t *gotest.T) {
		t.It("works", func(t *gotest.T) { gotest.True(t, true) })
	})
	t.When("a/c grouping", func(t *gotest.T) {
		t.It("also works", func(t *gotest.T) { gotest.True(t, true) })
	})
	t.When("https:// URI", func(t *gotest.T) {
		t.It("stays one level", func(t *gotest.T) { gotest.True(t, true) })
	})
}

// Labels that begin or end with a slash, beside a When whose own label ends in one.
func (s *WalkerTestSuite) TestWholeNames(t *gotest.T) {
	t.It("gets ./", func(t *gotest.T) { gotest.True(t, true) })
	t.It("/abs/pkg", func(t *gotest.T) { gotest.True(t, true) })
	t.When("reads specs/", func(t *gotest.T) {
		t.It("holds", func(t *gotest.T) { gotest.True(t, true) })
	})
	t.It("b#01", func(t *gotest.T) { gotest.True(t, true) })
	t.It("a", func(t *gotest.T) { gotest.True(t, true) })
	t.It("a", func(t *gotest.T) { gotest.True(t, true) })
}

// go test counts a name by the whole path, so a sibling spelled like a
// nested path collides with it.
func (s *WalkerTestSuite) TestFullNameRepeat(t *gotest.T) {
	t.When("a", func(t *gotest.T) {
		t.It("b", func(t *gotest.T) { gotest.True(t, true) })
	})
	t.It("a/b", func(t *gotest.T) { gotest.True(t, true) })
}

func (s *WalkerTestSuite) TestRepeatedLeaf(t *gotest.T) {
	t.It("x", func(t *gotest.T) { gotest.True(t, true) })
	t.It("x", func(t *gotest.T) { gotest.True(t, true) })
}

func (s *WalkerTestSuite) TestEmptyLabel(t *gotest.T) {
	t.It("", func(t *gotest.T) { gotest.True(t, true) })
}

func (s *WalkerTestSuite) TestExplicitNumber(t *gotest.T) {
	t.It("a#01", func(t *gotest.T) { gotest.True(t, true) })
}

// A written "a#01" takes that number from a later repeat of "a".
func (s *WalkerTestSuite) TestExplicitNumberFirst(t *gotest.T) {
	t.It("a#01", func(t *gotest.T) { gotest.True(t, true) })
	t.It("a", func(t *gotest.T) { gotest.True(t, true) })
	t.It("a", func(t *gotest.T) { gotest.True(t, true) })
}

// A written "a#01" after a repeat of "a" already took that number.
func (s *WalkerTestSuite) TestExplicitNumberLast(t *gotest.T) {
	t.It("a", func(t *gotest.T) { gotest.True(t, true) })
	t.It("a", func(t *gotest.T) { gotest.True(t, true) })
	t.It("a#01", func(t *gotest.T) { gotest.True(t, true) })
}

func (s *WalkerTestSuite) TestRepeatedRows(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []keyedRow{{"r", 1}, {"r", 2}}) {
		sub.It("y", func(it *gotest.T) { gotest.NotZero(it, tc.N) })
	}
}

type keyedRow struct {
	Desc string
	N    int
}

func (s *WalkerTestSuite) TestKeyedTable(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []keyedRow{
		{Desc: "negative", N: -1},
		{Desc: "zero", N: 0},
	}) {
		sub.It("classifies", func(it *gotest.T) { gotest.NotZero(it, tc.Desc) })
	}
}

type positionalRow struct {
	Name string
	N    int
}

func (s *WalkerTestSuite) TestPositionalTable(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []positionalRow{
		{"too short", 1},
		{"long enough", 2},
	}) {
		sub.It("checks the length", func(it *gotest.T) { gotest.NotZero(it, tc.N) })
	}
}

type namelessRow struct{ N int }

// Nothing names these rows, so go test falls back to the index.
func (s *WalkerTestSuite) TestUnnamedTable(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []namelessRow{{1}, {2}}) {
		sub.It("still runs", func(it *gotest.T) { gotest.NotZero(it, tc.N) })
	}
}

var rowsFromAVariable = []keyedRow{{Desc: "invisible", N: 1}}

func (s *WalkerTestSuite) TestNonLiteralTable(t *gotest.T) {
	for sub, tc := range gotest.Each(t, rowsFromAVariable) {
		sub.It("cannot be enumerated", func(it *gotest.T) { gotest.NotZero(it, tc.N) })
	}
}

func (s *WalkerTestSuite) TestConditional(t *gotest.T) {
	t.It("is always declared", func(t *gotest.T) { gotest.True(t, true) })
	if len(rowsFromAVariable) > 0 {
		t.It("is declared only sometimes", func(t *gotest.T) { gotest.True(t, true) })
	}
}

func (s *WalkerTestSuite) TestComputedDescription(t *gotest.T) {
	name := "built at run time"
	t.It(name, func(t *gotest.T) { gotest.True(t, true) })
}

func (s *WalkerTestSuite) TestNoBehaviorsAtAll(t *gotest.T) {
	gotest.True(t, true)
}

const localName = "local name"

type typedName string

const typedConst typedName = "typed name"

const prefix = "pre "

type bothRow struct {
	Desc string
	Name string
}

type intNameRow struct {
	Name int
}

var runtimeName = "known late"

// The type checker knows each of these rows' names.
func (s *WalkerTestSuite) TestConstantRows(t *gotest.T) {
	for sub := range gotest.Each(t, []bothRow{
		{Name: names.PSuite},
		{Name: localName},
		{Name: prefix + "x"},
		{Desc: "", Name: "n"},
		{Desc: "d", Name: "n"},
	}) {
		sub.It("runs", func(it *gotest.T) {})
	}
}

type typedRow struct{ Name typedName }

// A field of a string kind counts, and the type checker knows its value.
func (s *WalkerTestSuite) TestTypedConstantRows(t *gotest.T) {
	for sub := range gotest.Each(t, []typedRow{{Name: typedConst}}) {
		sub.It("runs", func(it *gotest.T) {})
	}
}

// Only a run knows a variable's value, so the row is not listed.
func (s *WalkerTestSuite) TestVariableRow(t *gotest.T) {
	for sub := range gotest.Each(t, []bothRow{{Name: "first"}, {Name: runtimeName}, {Name: "third"}}) {
		sub.It("runs", func(it *gotest.T) {})
	}
}

// A Name that is not a string kind does not name the row.
func (s *WalkerTestSuite) TestIntNameRow(t *gotest.T) {
	for sub := range gotest.Each(t, []intNameRow{{Name: 3}}) {
		sub.It("runs", func(it *gotest.T) {})
	}
}

// A Desc a variable sets beats a Name the reader could read.
func (s *WalkerTestSuite) TestVariableDescBeatsName(t *gotest.T) {
	for sub := range gotest.Each(t, []bothRow{{Desc: runtimeName, Name: "n"}}) {
		sub.It("runs", func(it *gotest.T) {})
	}
}
