package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"unicode"

	"github.com/mvrahden/go-test/internal/protocol"
)

// Behaviors are the `When`/`It` blocks a test method declares. They are the
// unit a developer writes in and reads, but until now they existed only as a
// runtime artifact: the only way to learn a suite's behaviors was to execute
// it. Reading them from source is the same act discovery already performs for
// suites and methods, one level deeper — a declaration is being enumerated,
// not an outcome predicted.
//
// Extraction is deliberately a partial function. Where a behavior's name or
// existence depends on runtime values, the walker records that it could not
// enumerate rather than guessing, and the consumer is expected to say so.

type BehaviorKind int

const (
	// BehaviorWhen is a `t.When(...)` block: a grouping node with children.
	BehaviorWhen BehaviorKind = iota
	// BehaviorIt is a `t.It(...)` block: a leaf behavior.
	BehaviorIt
	// BehaviorEach is one row of a `gotest.Each` table, named from the entry's
	// Desc or Name field exactly as the runtime names it.
	BehaviorEach
)

type Behavior struct {
	// Name is the subtest segment go test will produce: the same sanitisation
	// the testing package applies, plus the "#01" it appends to a name that
	// repeats among its siblings. A description containing a slash is split
	// into one Behavior per level, because that is what go test does with it.
	// All of this has to match byte for byte, because it is how a
	// statically-known behavior is reconciled with the one observed at run time.
	Name     string
	Display  string
	Kind     BehaviorKind
	Line     int
	Children []*Behavior
}

// MethodSpec is the behavior tree of a single test method. Complete reports
// whether the tree is exhaustive: when false, the method declares behaviors
// this walker cannot see, and Notes says which construct defeated it.
type MethodSpec struct {
	Behaviors []*Behavior
	Complete  bool
	Notes     []string
}

const gotestPkgPath = "github.com/mvrahden/go-test/pkg/gotest"

// BehaviorsOf walks a test method body and returns the behaviors it declares.
func (ts *TestSuiteSpec) BehaviorsOf(m *TestSuiteMethod) MethodSpec {
	fd, ok := m.n.(*ast.FuncDecl)
	if !ok || fd.Body == nil {
		return MethodSpec{Complete: false, Notes: []string{"method body unavailable"}}
	}
	w := &behaviorWalker{
		info:     ts.pkg.TypesInfo,
		fset:     ts.pkg.Fset,
		complete: true,
	}
	behaviors := w.walkBlock(fd.Body)
	return MethodSpec{Behaviors: behaviors, Complete: w.complete, Notes: w.notes}
}

type behaviorWalker struct {
	info     *types.Info
	fset     *token.FileSet
	complete bool
	notes    []string
}

func (w *behaviorWalker) incomplete(pos token.Pos, format string, args ...any) {
	w.complete = false
	note := fmt.Sprintf(format, args...)
	if w.fset != nil && pos.IsValid() {
		note = fmt.Sprintf("%s: %s", w.fset.Position(pos), note)
	}
	if len(w.notes) < 16 {
		w.notes = append(w.notes, note)
	}
}

func (w *behaviorWalker) walkBlock(block *ast.BlockStmt) []*Behavior {
	var out []*Behavior
	for _, stmt := range block.List {
		switch s := stmt.(type) {
		case *ast.ExprStmt:
			if b := w.behaviorCall(s.X); b != nil {
				out = append(out, b)
				continue
			}
			w.flagHidden(s)
		case *ast.RangeStmt:
			if rows := w.eachRows(s); rows != nil {
				out = append(out, rows...)
				continue
			}
			// A loop that is not a recognised Each table may still emit
			// behaviors, and how many depends on values we do not have.
			w.flagHidden(s)
		default:
			w.flagHidden(stmt)
		}
	}
	return resolveSiblings(out)
}

// resolveSiblings turns the behaviors written in one block into the subtests go
// test will actually produce for them. Two rules of the testing package apply
// here and neither is visible in the source: a name that repeats among its
// siblings gains a "#01" suffix, and a name containing a slash becomes one
// subtest level per segment. Skipping either would put a behavior in the tree
// under a name no run will ever report.
func resolveSiblings(in []*Behavior) []*Behavior {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]int, len(in))
	var out []*Behavior
	for _, b := range in {
		// The testing package uniquifies against the name as written, before
		// any slash in it is read as a separator, so uniquing comes first.
		name := b.Name
		if n := seen[name]; n > 0 {
			b.Name = fmt.Sprintf("%s#%02d", name, n)
		}
		seen[name]++
		out = insertPath(out, splitSegments(b.Name), splitSegments(b.Display), b)
	}
	return out
}

// insertPath places a behavior at the end of its segment path, reusing a
// grouping node a sibling already created. Two behaviors written as "a/b" and
// "a/c" are one node with two children at run time, not two nodes that happen
// to share a prefix.
func insertPath(out []*Behavior, segments, displays []string, leaf *Behavior) []*Behavior {
	if len(segments) <= 1 {
		return append(out, leaf)
	}
	head := segments[0]
	var node *Behavior
	for _, existing := range out {
		if existing.Name == head {
			node = existing
			break
		}
	}
	if node == nil {
		node = &Behavior{
			Name:    head,
			Display: displayAt(displays, 0, head),
			Kind:    BehaviorWhen,
			Line:    leaf.Line,
		}
		out = append(out, node)
	}
	rest := segments[1:]
	if len(rest) == 1 {
		leaf.Name = rest[0]
		leaf.Display = displayAt(displays, 1, rest[0])
	}
	node.Children = insertPath(node.Children, rest, tailFrom(displays, 1), leaf)
	return out
}

// splitSegments cuts a name at the separator go test uses between subtest
// levels, by the same rule the stream parser applies. Sanitisation never adds
// or removes a slash, so the description and the subtest name split into the
// same number of pieces.
func splitSegments(s string) []string {
	segments := protocol.SplitTestPath(s)
	if len(segments) == 0 {
		return []string{s}
	}
	return segments
}

func displayAt(displays []string, i int, fallback string) string {
	if i < len(displays) {
		return displays[i]
	}
	return fallback
}

func tailFrom(displays []string, i int) []string {
	if i < len(displays) {
		return displays[i:]
	}
	return nil
}

// flagHidden marks the tree incomplete if a construct we do not model
// nevertheless contains behavior calls. Statements with no behavior calls
// beneath them — assertions, setup, plain Go — are silently fine.
func (w *behaviorWalker) flagHidden(node ast.Node) {
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "When", "It":
			if w.isGotestT(sel.X) {
				w.incomplete(call.Pos(), "%s inside a construct whose behaviors depend on runtime values", sel.Sel.Name)
				return false
			}
		case "Each":
			if w.isGotestFunc(sel, "Each") {
				w.incomplete(call.Pos(), "Each over entries that are not a literal table")
				return false
			}
		}
		return true
	})
}

// behaviorCall recognises `x.When("...", func(y *gotest.T) {...})` and the
// same shape for It. Anything that is a behavior call but does not match this
// shape marks the tree incomplete rather than being dropped silently.
func (w *behaviorWalker) behaviorCall(expr ast.Expr) *Behavior {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	var kind BehaviorKind
	switch sel.Sel.Name {
	case "When":
		kind = BehaviorWhen
	case "It":
		kind = BehaviorIt
	default:
		return nil
	}
	if !w.isGotestT(sel.X) {
		return nil
	}
	if len(call.Args) < 2 {
		w.incomplete(call.Pos(), "%s call with unexpected arguments", sel.Sel.Name)
		return nil
	}
	desc, ok := stringLiteral(call.Args[0])
	if !ok {
		w.incomplete(call.Args[0].Pos(), "%s description is not a string literal", sel.Sel.Name)
		return nil
	}

	behavior := &Behavior{
		Name:    SubtestName(desc),
		Display: desc,
		Kind:    kind,
		Line:    w.line(call.Pos()),
	}

	fn, ok := call.Args[1].(*ast.FuncLit)
	if !ok || fn.Body == nil {
		w.incomplete(call.Args[1].Pos(), "%s body is not a function literal", sel.Sel.Name)
		return behavior
	}
	behavior.Children = w.walkBlock(fn.Body)
	return behavior
}

// eachRows enumerates `for sub, tc := range gotest.Each(t, []T{...})`. The
// runtime names each row from its Desc or Name field, falling back to the
// index, so a literal table is fully determined.
func (w *behaviorWalker) eachRows(s *ast.RangeStmt) []*Behavior {
	call, ok := s.X.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !w.isGotestFunc(sel, "Each") {
		return nil
	}
	if len(call.Args) < 2 {
		return nil
	}
	composite, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		return nil // caller flags it as incomplete
	}

	// Behaviors declared in the loop body run once per row, so they are the
	// children of every row rather than siblings of the table.
	var perRow []*Behavior
	if s.Body != nil {
		perRow = w.walkBlock(s.Body)
	}

	rows := make([]*Behavior, 0, len(composite.Elts))
	for i, elt := range composite.Elts {
		name := w.eachEntryName(elt, i)
		rows = append(rows, &Behavior{
			Name:     SubtestName(name),
			Display:  name,
			Kind:     BehaviorEach,
			Line:     w.line(elt.Pos()),
			Children: cloneBehaviors(perRow),
		})
	}
	return rows
}

// eachEntryName mirrors the runtime's eachEntryName: the Desc field wins, then
// Name, then the index. Both the keyed and the positional literal forms are
// resolved, because a table written as `{"too short", ...}` names its rows just
// as surely as one written as `{Desc: "too short"}`.
func (w *behaviorWalker) eachEntryName(elt ast.Expr, index int) string {
	composite, ok := elt.(*ast.CompositeLit)
	if !ok {
		return fmt.Sprintf("#%d", index)
	}

	for _, field := range []string{"Desc", "Name"} {
		for _, e := range composite.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != field {
				continue
			}
			if value, ok := stringLiteral(kv.Value); ok && value != "" {
				return value
			}
		}
	}

	if st := w.structOf(composite); st != nil {
		for _, field := range []string{"Desc", "Name"} {
			for i := 0; i < st.NumFields() && i < len(composite.Elts); i++ {
				if st.Field(i).Name() != field {
					continue
				}
				if _, keyed := composite.Elts[i].(*ast.KeyValueExpr); keyed {
					continue
				}
				if value, ok := stringLiteral(composite.Elts[i]); ok && value != "" {
					return value
				}
			}
		}
	}

	return fmt.Sprintf("#%d", index)
}

func (w *behaviorWalker) structOf(expr ast.Expr) *types.Struct {
	if w.info == nil {
		return nil
	}
	typ := w.info.TypeOf(expr)
	if typ == nil {
		return nil
	}
	st, _ := typ.Underlying().(*types.Struct)
	return st
}

// cloneBehaviors gives each table row its own subtree; sharing the nodes would
// alias rows that the runtime keeps entirely separate.
func cloneBehaviors(in []*Behavior) []*Behavior {
	if len(in) == 0 {
		return nil
	}
	out := make([]*Behavior, 0, len(in))
	for _, b := range in {
		clone := *b
		clone.Children = cloneBehaviors(b.Children)
		out = append(out, &clone)
	}
	return out
}

func (w *behaviorWalker) isGotestT(expr ast.Expr) bool {
	if w.info == nil {
		return false
	}
	typ := w.info.TypeOf(expr)
	if typ == nil {
		return false
	}
	ptr, ok := types.Unalias(typ).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Name() == "T" &&
		obj.Pkg() != nil && obj.Pkg().Path() == gotestPkgPath
}

func (w *behaviorWalker) isGotestFunc(sel *ast.SelectorExpr, name string) bool {
	if sel.Sel.Name != name || w.info == nil {
		return false
	}
	obj, ok := w.info.Uses[sel.Sel].(*types.Func)
	if !ok || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == gotestPkgPath
}

func (w *behaviorWalker) line(pos token.Pos) int {
	if w.fset == nil || !pos.IsValid() {
		return 0
	}
	return w.fset.Position(pos).Line
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// SubtestName applies the same rewriting the testing package uses for subtest
// names. A statically derived name that does not match the runtime one byte
// for byte would produce two tree entries for one behavior, so this is the
// contract between the two.
func SubtestName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case isSubtestSpace(r):
			b.WriteByte('_')
		case !strconv.IsPrint(r):
			quoted := strconv.QuoteRune(r)
			b.WriteString(quoted[1 : len(quoted)-1])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isSubtestSpace(r rune) bool {
	if r < utf8RuneSelf {
		return r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r'
	}
	return unicode.IsSpace(r)
}

const utf8RuneSelf = 0x80
