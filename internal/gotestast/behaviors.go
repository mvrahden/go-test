package gotestast

import (
	"fmt"
	"go/ast"
	"go/constant"
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
	// Name is the subtest level go test will produce: the same sanitisation
	// the testing package applies, plus the "#01" it appends to a name whose
	// full path repeats within the method. A slash in it stays in it. All of
	// this has to match byte for byte, because it is how a statically-known
	// behavior is reconciled with the one observed at run time.
	Name     string
	Display  string
	Kind     BehaviorKind
	Line     int
	Children []*Behavior
	// added is the length of the repeat suffix this reader appended to Name.
	added int
}

// Written is Name without the repeat suffix this reader added for a repeated
// full name. A suffix the label itself holds is part of what was written.
func (b *Behavior) Written() string { return b.Name[:len(b.Name)-b.added] }

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
		used:     map[string]int{},
	}
	behaviors := w.walkBlock(fd.Body)
	w.name(nil, behaviors)
	return MethodSpec{Behaviors: behaviors, Complete: w.complete, Notes: w.notes}
}

type behaviorWalker struct {
	info     *types.Info
	fset     *token.FileSet
	complete bool
	notes    []string
	// used counts each full name inside the method, as testing's subNames does.
	used map[string]int
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

// name gives each behavior the name go test reports, in the order the calls
// run: a call is named before its children, its children before the next
// sibling. Pre-order over the tree is that order, table rows included.
func (w *behaviorWalker) name(parent []string, behaviors []*Behavior) {
	for _, b := range behaviors {
		written := b.Name
		b.Name = w.unique(parent, written)
		// An empty label has only its number to show, as a run shows it.
		if written != "" {
			b.added = len(b.Name) - len(written)
		}
		w.name(append(parent[:len(parent):len(parent)], b.Name), b.Children)
	}
}

// unique ports testing's matcher.unique: it counts each full name, so a
// repeat gains "#01", an empty name "#00", and a written "#NN" that an
// earlier repeat already took is numbered again. The own level is returned.
func (w *behaviorWalker) unique(parent []string, sub string) string {
	// The method itself is the root, so every key starts with "/".
	prefix := "/"
	for _, level := range parent {
		prefix += level + "/"
	}
	base := prefix + sub
	for {
		n := w.used[base]
		w.used[base] = n + 1
		if n == 0 && sub != "" {
			stem, nn := parseSubtestNumber(base)
			if len(stem) < len(base) && nn < w.used[stem] {
				continue
			}
			return sub
		}
		name := fmt.Sprintf("%s#%02d", base, n)
		if w.used[name] != 0 {
			continue
		}
		return name[len(prefix):]
	}
}

// parseSubtestNumber ports testing's: it splits a "#NN" suffix that "%02d"
// could have produced from the name before it. "#00" counts only after an
// empty name.
func parseSubtestNumber(s string) (prefix string, nn int) {
	i := strings.LastIndex(s, "#")
	if i < 0 {
		return s, 0
	}
	prefix, suffix := s[:i], s[i+1:]
	if len(suffix) < 2 || (len(suffix) > 2 && suffix[0] == '0') {
		return s, 0
	}
	if suffix == "00" && !strings.HasSuffix(prefix, "/") {
		return s, 0
	}
	n, err := strconv.ParseInt(suffix, 10, 32)
	if err != nil || n < 0 {
		return s, 0
	}
	return prefix, int(n)
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
		name, known := w.eachEntryName(elt, i)
		if !known {
			w.incomplete(elt.Pos(), "Each row name is not a constant")
			continue
		}
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

// eachEntryName mirrors the runtime's eachEntryName: a Desc field of a string
// kind and not empty wins, then Name, then the index. Keyed and positional
// literals both resolve, and a value is known when it is a string literal or
// any constant the type checker evaluated. known is false when a field the
// runtime would read holds a value only a run has, and the row can not be
// named.
func (w *behaviorWalker) eachEntryName(elt ast.Expr, index int) (name string, known bool) {
	byIndex := fmt.Sprintf("#%d", index)
	composite, isLit := elt.(*ast.CompositeLit)
	st := w.structOf(elt)
	if st == nil {
		if w.info != nil && w.info.TypeOf(elt) != nil {
			return byIndex, true // rows that are not structs are named by index
		}
		if !isLit {
			return "", false
		}
	}
	if !isLit {
		return "", false // a struct value the source does not spell out
	}

	for _, field := range []string{"Desc", "Name"} {
		expr, ok := eachField(composite, st, field)
		if !ok {
			continue
		}
		if st != nil && !isStringField(st, field) {
			continue
		}
		value, ok := w.stringConstant(expr)
		if !ok {
			return "", false
		}
		if value != "" {
			return value, true
		}
	}
	return byIndex, true
}

// eachField is the expression a literal sets a field to, keyed or positional.
func eachField(composite *ast.CompositeLit, st *types.Struct, field string) (ast.Expr, bool) {
	for i, e := range composite.Elts {
		if kv, keyed := e.(*ast.KeyValueExpr); keyed {
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == field {
				return kv.Value, true
			}
			continue
		}
		if st != nil && i < st.NumFields() && st.Field(i).Name() == field {
			return e, true
		}
	}
	return nil, false
}

func isStringField(st *types.Struct, field string) bool {
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Name() == field {
			basic, ok := st.Field(i).Type().Underlying().(*types.Basic)
			return ok && basic.Info()&types.IsString != 0
		}
	}
	return false
}

// stringConstant is the value of a string literal or of any string constant
// the type checker evaluated.
func (w *behaviorWalker) stringConstant(expr ast.Expr) (string, bool) {
	if w.info != nil {
		if tv, ok := w.info.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
			return constant.StringVal(tv.Value), true
		}
	}
	return stringLiteral(expr)
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
