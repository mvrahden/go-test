package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"unicode"

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
	// Name is the subtest segment go test will produce, with the same
	// sanitisation the testing package applies. It has to match byte for byte,
	// because it is how a statically-known behavior is reconciled with the one
	// observed at run time.
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
	return out
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

	rows := make([]*Behavior, 0, len(composite.Elts))
	for i, elt := range composite.Elts {
		name, ok := eachEntryLiteralName(elt)
		if !ok {
			name = fmt.Sprintf("#%d", i)
		}
		rows = append(rows, &Behavior{
			Name:    SubtestName(name),
			Display: name,
			Kind:    BehaviorEach,
			Line:    w.line(elt.Pos()),
		})
	}
	// Behaviors declared inside the loop body would be nested under each row;
	// that shape is not modelled, so say so rather than under-report.
	if s.Body != nil {
		for _, stmt := range s.Body.List {
			w.flagHiddenNested(stmt)
		}
	}
	return rows
}

func (w *behaviorWalker) flagHiddenNested(stmt ast.Stmt) {
	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok &&
			(sel.Sel.Name == "When" || sel.Sel.Name == "It") && w.isGotestT(sel.X) {
			w.incomplete(call.Pos(), "%s nested inside an Each row", sel.Sel.Name)
			return false
		}
		return true
	})
}

// eachEntryLiteralName mirrors eachEntryName: the Desc field wins, then Name.
func eachEntryLiteralName(elt ast.Expr) (string, bool) {
	composite, ok := elt.(*ast.CompositeLit)
	if !ok {
		return "", false
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
				return value, true
			}
		}
	}
	return "", false
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
