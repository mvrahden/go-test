package mutate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// A mutable node is a site; each site holds layers: a structural one that
// renders the node from its children (an operator swapped, a literal
// picked) and wrapping ones around the text below (a condition negated, a
// statement removed, a return put before it). Every rendering keeps the
// source's newlines, so a line of the schemata is the line of the source
// and a compiler error names the mutant's own line.

type layer struct {
	op        string
	alts      []string // the mutated spelling of each mutant
	ops       []string // the operator of each alt, when it differs from op
	orig      string
	line, col int
	build     func(g *gen, ids []int, inner string) string
	funcName  string
}

type site struct {
	node       ast.Node
	start, end int
	depth      int
	struc      *layer
	wraps      []*layer
	order      int
	idsets     [][]int
}

type gen struct {
	pkg   *packages.Package
	file  *ast.File
	src   []byte
	lines map[int]bool
	path  string
	opt   Options
	// picks are the basic types a literal pick is spelled for, shared by
	// the package's files.
	picks map[string]bool

	sites    map[ast.Node]*site
	sorted   []*site
	funcs    []string
	muts     []Mutant
	rendered bool
}

func (g *gen) fset() *token.FileSet   { return g.pkg.Fset }
func (g *gen) info() *types.Info      { return g.pkg.TypesInfo }
func (g *gen) off(p token.Pos) int    { return g.fset().Position(p).Offset }
func (g *gen) lineOf(p token.Pos) int { return g.fset().Position(p).Line }
func (g *gen) colOf(p token.Pos) int  { return g.fset().Position(p).Column }
func (g *gen) text(n ast.Node) string { return string(g.src[g.off(n.Pos()):g.off(n.End())]) }

// multiline reports a node spanning lines: a layer that writes its text
// twice would add lines.
func (g *gen) multiline(n ast.Node) bool { return strings.Contains(g.text(n), "\n") }

func (g *gen) site(n ast.Node, depth int) *site {
	s := g.sites[n]
	if s == nil {
		s = &site{node: n, start: g.off(n.Pos()), end: g.off(n.End()), depth: depth}
		g.sites[n] = s
	}
	return s
}

func (g *gen) funcName() string {
	if len(g.funcs) == 0 {
		return ""
	}
	return g.funcs[len(g.funcs)-1]
}

// collect walks the file and records each mutable node on a changed line.
// Constants, imports, types, signatures, array lengths and struct tags hold
// no run-time choice and are not entered.
func (g *gen) collect() {
	g.sites = map[ast.Node]*site{}
	var stack []ast.Node
	ast.Inspect(g.file, func(n ast.Node) bool {
		if n == nil {
			if top := stack[len(stack)-1]; isFunc(top) {
				if _, ok := top.(*ast.FuncDecl); ok {
					g.funcs = g.funcs[:len(g.funcs)-1]
				}
			}
			stack = stack[:len(stack)-1]
			return true
		}
		depth := len(stack)
		var parent ast.Node
		if depth > 0 {
			parent = stack[depth-1]
		}
		switch x := n.(type) {
		case *ast.GenDecl:
			if x.Tok != token.VAR {
				return false
			}
		case *ast.ArrayType, *ast.Field, *ast.FuncType, *ast.ImportSpec:
			return false
		}
		stack = append(stack, n)
		switch x := n.(type) {
		case *ast.FuncDecl:
			g.funcs = append(g.funcs, funcDeclName(x))
			if x.Body != nil {
				g.earlyReturn(x.Type, x.Body, depth+2)
			}
		case *ast.FuncLit:
			g.earlyReturn(x.Type, x.Body, depth+2)
		case *ast.BinaryExpr:
			if g.lines[g.lineOf(x.OpPos)] {
				g.binary(x, depth)
			}
		case *ast.BasicLit:
			if g.lines[g.lineOf(x.Pos())] && !isKey(parent, x) {
				g.literal(x, depth)
			}
		case *ast.Ident:
			if g.lines[g.lineOf(x.Pos())] {
				g.boolIdent(x, depth)
			}
		case *ast.IfStmt:
			g.cond(x.Cond, depth+1)
		case *ast.ForStmt:
			if x.Cond != nil {
				g.cond(x.Cond, depth+1)
			}
		case *ast.BlockStmt:
			g.statements(x.List, depth+1)
		case *ast.CaseClause:
			g.statements(x.Body, depth+1)
		case *ast.CommClause:
			g.statements(x.Body, depth+1)
		}
		return true
	})
	for _, s := range g.sites {
		g.sorted = append(g.sorted, s)
	}
	sort.Slice(g.sorted, func(i, j int) bool {
		a, b := g.sorted[i], g.sorted[j]
		if a.start != b.start {
			return a.start < b.start
		}
		if a.end != b.end {
			return a.end > b.end
		}
		return a.depth < b.depth
	})
	for i, s := range g.sorted {
		s.order = i
	}
}

func isFunc(n ast.Node) bool {
	switch n.(type) {
	case *ast.FuncDecl, *ast.FuncLit:
		return true
	}
	return false
}

// funcDeclName is a function's name, a method's as Type.Method.
func funcDeclName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	t := d.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
			continue
		case *ast.IndexExpr:
			t = x.X
			continue
		case *ast.IndexListExpr:
			t = x.X
			continue
		case *ast.Ident:
			return x.Name + "." + d.Name.Name
		}
		return d.Name.Name
	}
}

// isKey reports a literal that keys a composite literal: an array's key
// must stay constant.
func isKey(parent ast.Node, lit *ast.BasicLit) bool {
	kv, ok := parent.(*ast.KeyValueExpr)
	return ok && kv.Key == lit
}

func (g *gen) constant(e ast.Expr) bool {
	tv, ok := g.info().Types[e]
	return !ok || tv.Value != nil
}

func (g *gen) addLayer(s *site, structural bool, l *layer) {
	l.funcName = g.funcName()
	if structural {
		s.struc = l
		return
	}
	s.wraps = append(s.wraps, l)
}

// compareAlts are each ordering's boundary shift and negation.
var compareAlts = map[token.Token][2]token.Token{
	token.LSS: {token.LEQ, token.GEQ},
	token.LEQ: {token.LSS, token.GTR},
	token.GTR: {token.GEQ, token.LEQ},
	token.GEQ: {token.GTR, token.LSS},
}

var swapped = map[token.Token]token.Token{
	token.EQL: token.NEQ, token.NEQ: token.EQL,
	token.LAND: token.LOR, token.LOR: token.LAND,
}

// binary swaps a comparison's or a logical operator. A constant expression
// has no run-time choice; an operand spanning lines would be written twice.
func (g *gen) binary(x *ast.BinaryExpr, depth int) {
	if g.constant(x) || g.multiline(x) {
		return
	}
	line, col := g.lineOf(x.OpPos), g.colOf(x.OpPos)
	var alts []token.Token
	var ops []string
	op := OpNegate
	switch x.Op {
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
		a := compareAlts[x.Op]
		alts, ops, op = []token.Token{a[0], a[1]}, []string{OpBoundary, OpNegate}, OpBoundary
	case token.EQL, token.NEQ:
		alts = []token.Token{swapped[x.Op]}
	case token.LAND, token.LOR:
		alts, op = []token.Token{swapped[x.Op]}, OpLogical
	default:
		return
	}
	texts := make([]string, len(alts))
	for i, a := range alts {
		texts[i] = shorten(g.text(x.X) + " " + a.String() + " " + g.text(x.Y))
	}
	g.addLayer(g.site(x, depth), true, &layer{op: op, ops: ops, orig: shorten(g.text(x)), alts: texts, line: line, col: col,
		build: func(g *gen, ids []int, _ string) string { return g.chain(ids, x, alts) }})
}

// chain picks among x's operator spellings without evaluating an operand
// twice: only the active branch runs, short-circuiting as written.
func (g *gen) chain(ids []int, x *ast.BinaryExpr, alts []token.Token) string {
	l, r := g.emitNode(x.X), g.emitNode(x.Y)
	var b strings.Builder
	b.WriteString("(")
	var none []string
	for i, op := range alts {
		if ids[i] == 0 {
			continue
		}
		fmt.Fprintf(&b, "gotestMutantID == %d && ((%s) %s (%s)) || ", ids[i], l, op, r)
		none = append(none, fmt.Sprintf("gotestMutantID != %d", ids[i]))
	}
	b.WriteString(strings.Join(none, " && ") + " && ")
	fmt.Fprintf(&b, "((%s) %s (%s)))", l, x.Op, r)
	return b.String()
}

// literal flips an integer 0 and 1 of a basic type and, when asked for, a
// string. A literal of a named type, an untyped one or one of a type
// parameter cannot be spelled as a choice and is left.
func (g *gen) literal(x *ast.BasicLit, depth int) {
	tv, ok := g.info().Types[x]
	if !ok {
		return
	}
	b, isBasic := tv.Type.(*types.Basic)
	if !isBasic || b.Info()&types.IsUntyped != 0 {
		return
	}
	var alt, op string
	switch x.Kind {
	case token.INT:
		if b.Info()&types.IsInteger == 0 {
			return
		}
		n, err := strconv.ParseInt(strings.ReplaceAll(x.Value, "_", ""), 0, 64)
		if err != nil || (n != 0 && n != 1) {
			return
		}
		alt, op = strconv.FormatInt(1-n, 10), OpLiteral
	case token.STRING:
		if !g.opt.Strings || b.Kind() != types.String || g.multiline(x) {
			return
		}
		alt, op = `""`, OpString
		if s, err := strconv.Unquote(x.Value); err == nil && s == "" {
			alt = `"gotest-mutant"`
		}
	default:
		return
	}
	typ := types.Typ[b.Kind()].Name()
	g.addLayer(g.site(x, depth), true, &layer{op: op, orig: x.Value, alts: []string{alt}, line: g.lineOf(x.Pos()), col: g.colOf(x.Pos()),
		build: func(g *gen, ids []int, _ string) string {
			g.picks[typ] = true
			return fmt.Sprintf("gotestMutPick_%s(%d, %s, %s)", typ, ids[0], x.Value, alt)
		}})
}

// boolIdent flips the predeclared true and false.
func (g *gen) boolIdent(x *ast.Ident, depth int) {
	obj := g.info().Uses[x]
	if obj == nil || obj.Parent() != types.Universe || (x.Name != "true" && x.Name != "false") {
		return
	}
	alt, check := "false", "!="
	if x.Name == "false" {
		alt, check = "true", "=="
	}
	g.addLayer(g.site(x, depth), true, &layer{op: OpLiteral, orig: x.Name, alts: []string{alt}, line: g.lineOf(x.Pos()), col: g.colOf(x.Pos()),
		build: func(_ *gen, ids []int, _ string) string { return fmt.Sprintf("(gotestMutantID %s %d)", check, ids[0]) }})
}

// cond negates an if or for condition, unless it is a comparison, whose
// negation the comparison's own mutant makes.
func (g *gen) cond(c ast.Expr, depth int) {
	if !g.lines[g.lineOf(c.Pos())] || g.constant(c) {
		return
	}
	if be, ok := unparen(c).(*ast.BinaryExpr); ok {
		switch be.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			return
		}
	}
	text := shorten(g.text(c))
	g.addLayer(g.site(c, depth), false, &layer{op: OpCond, orig: text, alts: []string{"!(" + text + ")"}, line: g.lineOf(c.Pos()), col: g.colOf(c.Pos()),
		build: func(_ *gen, ids []int, inner string) string {
			return fmt.Sprintf("((%s) != (gotestMutantID == %d))", inner, ids[0])
		}})
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// statements removes the simple statements of a block or case body on
// changed lines: an expression statement but a panic, an assignment but a
// declaring one, an increment. A statement of an if, for or switch header
// cannot be wrapped and is left.
func (g *gen) statements(list []ast.Stmt, depth int) {
	for _, st := range list {
		line := g.lineOf(st.Pos())
		if !g.lines[line] {
			continue
		}
		switch x := st.(type) {
		case *ast.ExprStmt:
			if isPanic(x.X) {
				continue
			}
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				continue
			}
		case *ast.IncDecStmt:
		default:
			continue
		}
		g.addLayer(g.site(st, depth), false, &layer{op: OpDelete, orig: shorten(g.text(st)), alts: []string{""}, line: line, col: g.colOf(st.Pos()),
			build: func(_ *gen, ids []int, inner string) string {
				return fmt.Sprintf("if gotestMutantID != %d { %s }", ids[0], inner)
			}})
	}
}

func isPanic(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "panic"
}

// earlyReturn puts a return of zero values before the first top-level
// statement of body that starts on a changed line. Only the top-level block
// takes a bare return safely: no named result is shadowed there yet.
func (g *gen) earlyReturn(ft *ast.FuncType, body *ast.BlockStmt, depth int) {
	for _, st := range body.List {
		line := g.lineOf(st.Pos())
		if !g.lines[line] {
			continue
		}
		if _, isLabel := st.(*ast.LabeledStmt); isLabel {
			return
		}
		ret, shown := "return", "return"
		if ft.Results != nil && len(ft.Results.List) > 0 && len(ft.Results.List[0].Names) == 0 {
			var zeros, texts []string
			for _, f := range ft.Results.List {
				zeros = append(zeros, "*new("+g.text(f.Type)+")")
				texts = append(texts, zeroText(g.info().TypeOf(f.Type), g.text(f.Type)))
			}
			ret, shown = "return "+strings.Join(zeros, ", "), "return "+strings.Join(texts, ", ")
		}
		if strings.Contains(ret, "\n") {
			return
		}
		g.addLayer(g.site(st, depth), false, &layer{op: OpReturn, orig: shorten(g.text(st)), alts: []string{shorten(shown)}, line: line, col: g.colOf(st.Pos()),
			build: func(_ *gen, ids []int, inner string) string {
				return fmt.Sprintf("if gotestMutantID == %d { %s }; %s", ids[0], ret, inner)
			}})
		return
	}
}

// zeroText is how a reader writes the zero value of t, spelled typ.
func zeroText(t types.Type, typ string) string {
	if _, param := t.(*types.TypeParam); param || t == nil {
		return "*new(" + typ + ")"
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return "false"
		case u.Info()&types.IsString != 0:
			return `""`
		case u.Info()&types.IsNumeric != 0:
			return "0"
		}
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return "nil"
	case *types.Struct, *types.Array:
		return typ + "{}"
	}
	return "*new(" + typ + ")"
}

func shorten(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 72 {
		return s[:69] + "..."
	}
	return s
}

// render writes the file with every site's layers, numbering mutants from
// next in source order and leaving out the dropped ones.
func (g *gen) render(next *int) ([]byte, []Mutant) {
	for _, s := range g.sorted {
		for _, l := range s.layers() {
			ids := make([]int, len(l.alts))
			for i, alt := range l.alts {
				id := *next
				*next++
				op := l.op
				if len(l.ops) > i {
					op = l.ops[i]
				}
				if g.opt.Drop[id] {
					continue
				}
				ids[i] = id
				g.muts = append(g.muts, Mutant{ID: id, Package: g.pkg.PkgPath, File: g.path, Line: l.line, Col: l.col, Func: l.funcName, Op: op, Original: l.orig, Mutated: alt})
				g.rendered = true
			}
			s.idsets = append(s.idsets, ids)
		}
	}
	return []byte(g.emitRange(0, len(g.src), -1)), g.muts
}

func (s *site) layers() []*layer {
	var out []*layer
	if s.struc != nil {
		out = append(out, s.struc)
	}
	return append(out, s.wraps...)
}

// emitNode renders a node with the sites inside it.
func (g *gen) emitNode(n ast.Node) string {
	if s, ok := g.sites[n]; ok {
		return g.renderSite(s)
	}
	return g.emitRange(g.off(n.Pos()), g.off(n.End()), -1)
}

// emitRange copies src[start:end], rendering each outermost site within it
// that sorts after index after.
func (g *gen) emitRange(start, end, after int) string {
	var b strings.Builder
	pos := start
	for _, s := range g.sorted[after+1:] {
		if s.start >= end {
			break
		}
		if s.start < pos || s.end > end {
			continue
		}
		b.Write(g.src[pos:s.start])
		b.WriteString(g.renderSite(s))
		pos = s.end
	}
	b.Write(g.src[pos:end])
	return b.String()
}

func (g *gen) renderSite(s *site) string {
	layers := s.layers()
	k := 0
	var inner string
	if s.struc != nil {
		if ids := s.idsets[0]; slices.ContainsFunc(ids, func(id int) bool { return id != 0 }) {
			inner = s.struc.build(g, ids, "")
		} else {
			inner = g.emitRange(s.start, s.end, s.order)
		}
		k = 1
	} else {
		inner = g.emitRange(s.start, s.end, s.order)
	}
	for ; k < len(layers); k++ {
		if ids := s.idsets[k]; slices.ContainsFunc(ids, func(id int) bool { return id != 0 }) {
			inner = layers[k].build(g, ids, inner)
		}
	}
	return inner
}

// helperFile declares the active mutant, read once from EnvMutant, and a
// pick for each basic type a literal is spelled in.
func helperFile(pkgName string, picks map[string]bool) []byte {
	var b strings.Builder
	b.WriteString(`// Code generated by gotest audit --mutate. DO NOT EDIT.

package ` + pkgName + `

import (
	gotestMutOS "os"
	gotestMutStrconv "strconv"
)

var gotestMutantID = func() int { n, _ := gotestMutStrconv.Atoi(gotestMutOS.Getenv("` + EnvMutant + `")); return n }()
`)
	for _, typ := range slices.Sorted(func(yield func(string) bool) {
		for k := range picks {
			if !yield(k) {
				return
			}
		}
	}) {
		fmt.Fprintf(&b, `
func gotestMutPick_%[1]s(id int, orig, mut %[1]s) %[1]s {
	if gotestMutantID == id {
		return mut
	}
	return orig
}
`, typ)
	}
	return []byte(b.String())
}
