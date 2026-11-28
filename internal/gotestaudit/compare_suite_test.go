package gotestaudit_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditCompareTestSuite: two records compare by what they declare, and a
// rename is told only where one behavior explains it.
type AuditCompareTestSuite struct{}

func (s *AuditCompareTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// leaf is an It with a body hash.
func leaf(name, hash string) gotestaudit.Behavior {
	return gotestaudit.Behavior{Kind: gotestaudit.KindIt, Name: name, Display: name, Hash: hash, Assertions: 1}
}

func test(name string, kids ...gotestaudit.Behavior) gotestaudit.Behavior {
	return gotestaudit.Behavior{Kind: gotestaudit.KindTest, Name: name, Display: name[4:], Hash: "own", Children: kids}
}

func recordOf(suites ...gotestaudit.Suite) *gotestaudit.Record {
	return &gotestaudit.Record{Schema: gotestaudit.Schema, Notes: []string{gotestaudit.FloorNote, gotestaudit.SkipNote}, Packages: []gotestaudit.Package{{Path: "m/p", Module: "m", Suites: suites}}}
}

func changes(doc *gotestaudit.Document) []string {
	var out []string
	for i := range doc.Behaviors {
		c := &doc.Behaviors[i]
		p := c.Head
		if p == nil {
			p = c.Base
		}
		out = append(out, c.Change+" "+c.Kind+" "+p.Path[len(p.Path)-1])
	}
	return out
}

func (s *AuditCompareTestSuite) TestRenames(t *gotest.T) {
	t.When("a suite is renamed and its tests are not", func(w *gotest.T) {
		w.It("lists the suite as reworded and none of its tests", func(it *gotest.T) {
			base := recordOf(gotestaudit.Suite{Name: "OldTestSuite", Display: "Old", Methods: []gotestaudit.Behavior{test("TestA", leaf("x", "1"))}})
			head := recordOf(gotestaudit.Suite{Name: "NewTestSuite", Display: "New", Methods: []gotestaudit.Behavior{test("TestA", leaf("x", "1"))}})
			doc := gotestaudit.Compare(base, head)
			gotest.Equal(it, []string{"reworded suite New"}, changes(&doc))
			gotest.Empty(it, doc.Findings)
		})
	})
	t.When("a test moves to another suite unchanged", func(w *gotest.T) {
		w.It("lists it as moved and its behaviors as they were", func(it *gotest.T) {
			base := recordOf(
				gotestaudit.Suite{Name: "ATestSuite", Display: "A", Methods: []gotestaudit.Behavior{test("TestKeep", leaf("k", "0")), test("TestGo", leaf("g", "2"))}},
				gotestaudit.Suite{Name: "BTestSuite", Display: "B", Methods: []gotestaudit.Behavior{test("TestStay", leaf("s", "3"))}},
			)
			head := recordOf(
				gotestaudit.Suite{Name: "ATestSuite", Display: "A", Methods: []gotestaudit.Behavior{test("TestKeep", leaf("k", "0"))}},
				gotestaudit.Suite{Name: "BTestSuite", Display: "B", Methods: []gotestaudit.Behavior{test("TestStay", leaf("s", "3")), test("TestGo", leaf("g", "2"))}},
			)
			doc := gotestaudit.Compare(base, head)
			gotest.Equal(it, []string{"moved test Go"}, changes(&doc))
		})
	})
	t.When("a test's behaviors are not all named in source", func(w *gotest.T) {
		w.It("compares it as one body and lists none of its behaviors", func(it *gotest.T) {
			before := test("TestA", leaf("x", "1"))
			before.Incomplete = true
			after := test("TestA", leaf("y", "9"))
			doc := gotestaudit.Compare(
				recordOf(gotestaudit.Suite{Name: "ATestSuite", Display: "A", Methods: []gotestaudit.Behavior{before}}),
				recordOf(gotestaudit.Suite{Name: "ATestSuite", Display: "A", Methods: []gotestaudit.Behavior{after}}),
			)
			gotest.Equal(it, []string{"changed test A"}, changes(&doc))
			gotest.Len(it, doc.Notes, 1)
		})
	})
}

func (s *AuditCompareTestSuite) TestClasses(t *gotest.T) {
	t.When("--fail-on names all", func(w *gotest.T) {
		w.It("takes every class but changed, in the document's order", func(it *gotest.T) {
			got, err := gotestaudit.ParseClasses("snapshot,all")
			gotest.NoError(it, err)
			gotest.Equal(it, []string{"removed", "excluded", "skip", "focus", "guard", "budget", "suppression", "snapshot", "assertions", "asserts-by"}, got)
		})
	})
	t.When("--fail-on names nothing", func(w *gotest.T) {
		w.It("is refused", func(it *gotest.T) {
			_, err := gotestaudit.ParseClasses(" , ")
			gotest.ErrorContains(it, err, "no finding class named")
		})
	})
}

func (s *AuditCompareTestSuite) TestTheCountedFunctions(t *gotest.T) {
	t.When("pkg/gotest declares its assertions", func(w *gotest.T) {
		w.It("counts exactly the exported functions of assertions.go and MatchSnapshot", func(it *gotest.T) {
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", "pkg", "gotest", "assertions.go"), nil, parser.SkipObjectResolution)
			gotest.NoError(it, err)
			want := []string{"MatchSnapshot"}
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.IsExported() {
					want = append(want, fd.Name.Name)
				}
			}
			var got []string
			for name := range gotestaudit.ExportAssertionFuncs {
				got = append(got, name)
			}
			slices.Sort(want)
			slices.Sort(got)
			gotest.Equal(it, want, got)
		})
	})
}

// classFindings are a document's findings as {Class, Subject, Base, Head}.
func classFindings(doc *gotestaudit.Document) [][4]string {
	var out [][4]string
	for i := range doc.Findings {
		f := &doc.Findings[i]
		out = append(out, [4]string{f.Class, f.Subject, f.Base, f.Head})
	}
	return out
}

// skipping is a test X of suite A whose body writes n skip calls.
func skipping(name string, n int) gotestaudit.Suite {
	b := test(name)
	b.Skips = n
	return gotestaudit.Suite{Name: "ATestSuite", Display: "A", Methods: []gotestaudit.Behavior{b}}
}

func (s *AuditCompareTestSuite) TestSkips(t *gotest.T) {
	for row, r := range gotest.Each(t, []struct {
		Desc       string
		base, head int
		findings   [][4]string
	}{
		{Desc: "a call added", base: 0, head: 1, findings: [][4]string{{"skip", "A / X", "no skip call", "1 skip call"}}},
		{Desc: "a second call added", base: 1, head: 3, findings: [][4]string{{"skip", "A / X", "1 skip call", "3 skip calls"}}},
		{Desc: "a call kept", base: 1, head: 1},
		{Desc: "a call dropped", base: 2, head: 1},
	}) {
		doc := gotestaudit.Compare(recordOf(skipping("TestX", r.base)), recordOf(skipping("TestX", r.head)))
		gotest.Equal(row, r.findings, classFindings(&doc))
	}
	t.When("the head adds a test whose body writes skip calls", func(w *gotest.T) {
		w.It("lists it as a skip weakening with no base value", func(it *gotest.T) {
			head := recordOf(gotestaudit.Suite{Name: "ATestSuite", Display: "A", Methods: []gotestaudit.Behavior{test("TestX"), skipping("TestY", 2).Methods[0]}})
			doc := gotestaudit.Compare(recordOf(skipping("TestX", 0)), head)
			gotest.Equal(it, [][4]string{{"skip", "A / Y", "", "2 skip calls"}}, classFindings(&doc))
			gotest.True(it, doc.Findings[0].Weakens)
		})
	})
	t.When("the base record holds no skip counts", func(w *gotest.T) {
		w.It("lists no skip finding and notes why", func(it *gotest.T) {
			base := recordOf(skipping("TestX", 0))
			base.Notes = []string{gotestaudit.FloorNote}
			doc := gotestaudit.Compare(base, recordOf(skipping("TestX", 1)))
			gotest.Empty(it, doc.Findings)
			gotest.Equal(it, []string{gotestaudit.NoSkipsNote}, doc.Notes)
		})
	})
}
