package gotestaudit

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/protocol"
)

// readSuite records one suite: its facts, its methods and their behaviors.
func readSuite(ts *gotestast.TestSuiteSpec, m module, external bool) Suite {
	fset := ts.Package().Fset
	r := &bodyReader{fset: fset, src: map[string][]byte{}}
	s := Suite{
		Name:      ts.Identifier(),
		External:  external,
		Location:  r.location(m, ts.TypeSpecPos(), ts.TypeSpecEnd()),
		Focused:   ts.IsFocused(),
		Excluded:  ts.IsExcluded(),
		Guard:     ts.HasGuard(),
		Parallel:  ts.IsMethodParallel(),
		Exclusive: ts.IsExclusive(),
		Group:     ts.ExclusiveGroup(),
		Budget:    readBudget(ts),
	}
	// The spec's tree names what a reader sees; the record shows the same.
	node := &gotestspec.Node{Name: "Test" + ts.Identifier()}
	var nodes []*gotestspec.Node
	add := func(meth *gotestast.TestSuiteMethod, kind string) {
		r.info = meth.TypesInfo()
		b, n := r.method(meth, kind, m)
		if decl := meth.Decl(); kind == KindTest && decl != nil && decl.Body != nil {
			spec := ts.BehaviorsOf(meth)
			b.Incomplete = !spec.Complete
			b.Children, n.Children = r.behaviors(spec.Behaviors, m)
			b.Hash, b.Assertions, b.AssertsBy, b.Skips = r.body([]ast.Node{decl.Body}, stmtsOf(spec.Behaviors))
		}
		s.Methods = append(s.Methods, b)
		nodes = append(nodes, n)
	}
	for _, meth := range ts.TestCases() {
		add(meth, KindTest)
	}
	for _, meth := range ts.Benchmarks() {
		add(meth, KindBenchmark)
	}
	for _, meth := range ts.Fuzzers() {
		add(meth, KindFuzz)
	}
	node.Children = nodes
	gotestspec.ClassifyRoots([]*gotestspec.Node{node})
	s.Display = node.Display
	for i := range s.Methods {
		copyDisplays(&s.Methods[i], nodes[i])
	}
	return s
}

// method records a method's own facts; a test's behaviors are added after.
func (r *bodyReader) method(meth *gotestast.TestSuiteMethod, kind string, m module) (Behavior, *gotestspec.Node) {
	name := meth.Identifier()
	b := Behavior{
		Kind:     kind,
		Name:     name,
		Focused:  meth.IsFocused(),
		Excluded: meth.IsExcluded(),
	}
	n := &gotestspec.Node{Name: name}
	if decl := meth.Decl(); decl != nil {
		b.Location = r.location(m, decl.Pos(), decl.End())
		if kind != KindTest && decl.Body != nil {
			b.Hash, b.Assertions, b.AssertsBy, b.Skips = r.body([]ast.Node{decl.Body}, nil)
		}
	}
	if kind == KindFuzz {
		// The run tree shapes fuzz targets itself; their label is the name.
		n.Kind = gotestspec.KindFuzz
		n.Display = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded), protocol.PrefixFuzz)
	}
	return b, n
}

// behaviors records the When, It and row blocks of a body.
func (r *bodyReader) behaviors(bs []*gotestast.Behavior, m module) ([]Behavior, []*gotestspec.Node) {
	out := make([]Behavior, 0, len(bs))
	nodes := make([]*gotestspec.Node, 0, len(bs))
	for _, b := range bs {
		rec := Behavior{Name: b.Name}
		node := &gotestspec.Node{Name: b.Name, SourceLabel: b.Display}
		var parts []ast.Node
		switch b.Kind {
		case gotestast.BehaviorWhen:
			rec.Kind, node.Vocab = KindWhen, gotestspec.VocabWhen
		case gotestast.BehaviorIt:
			rec.Kind, node.Vocab = KindIt, gotestspec.VocabIt
		default:
			rec.Kind, node.Vocab = KindRow, gotestspec.VocabEach
		}
		switch {
		case b.Entry != nil:
			parts = append(parts, b.Entry)
			rec.Location = r.location(m, b.Entry.Pos(), b.Entry.End())
		case b.Stmt != nil:
			rec.Location = r.location(m, b.Stmt.Pos(), b.Stmt.End())
		}
		if b.Body != nil {
			parts = append(parts, b.Body)
		}
		rec.Hash, rec.Assertions, rec.AssertsBy, rec.Skips = r.body(parts, stmtsOf(b.Children))
		rec.Children, node.Children = r.behaviors(b.Children, m)
		out = append(out, rec)
		nodes = append(nodes, node)
	}
	return out, nodes
}

// stmtsOf are the statements declaring bs, each once: a table's rows share
// their loop.
func stmtsOf(bs []*gotestast.Behavior) []ast.Node {
	var out []ast.Node
	seen := map[ast.Stmt]bool{}
	for _, b := range bs {
		if b.Stmt != nil && !seen[b.Stmt] {
			seen[b.Stmt] = true
			out = append(out, b.Stmt)
		}
	}
	return out
}

// copyDisplays copies each node's label onto its behavior.
func copyDisplays(b *Behavior, n *gotestspec.Node) {
	b.Display = n.Display
	for i := range b.Children {
		copyDisplays(&b.Children[i], n.Children[i])
	}
}

// location is the module-relative place from pos to end.
func (r *bodyReader) location(m module, pos, end token.Pos) Location {
	p, e := r.fset.Position(pos), r.fset.Position(end)
	if !p.IsValid() {
		return Location{}
	}
	if !e.IsValid() {
		e = p
	}
	return Location{File: m.rel(p.Filename), Line: p.Line, EndLine: e.Line}
}
