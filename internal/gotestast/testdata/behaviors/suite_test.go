// Source the behavior walker is read against. It lives under testdata so the
// go tool never builds it as part of the repository, and it is written to be
// adverse: every construct here is one the walker either has to model exactly
// or has to admit it cannot see.
package behaviors

import "github.com/mvrahden/go-test/pkg/gotest"

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

// A single slash is a level separator; a run of them is not.
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
