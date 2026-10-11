package migrate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"strings"
)

// transformFile migrates f in place for the unit's migrated suites, wherever
// in the unit they are declared, and reports whether it changed anything.
func (u *unit) transformFile(f *ast.File) bool {
	orig := map[*ast.Ident]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			orig[ident] = true
		}
		return true
	})
	changed := false

	// 1. Rename every identifier that resolves to a migrated suite type.
	// A field, variable or key spelled the same resolves elsewhere.
	ast.Inspect(f, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := u.info.Uses[ident]
		if obj == nil {
			obj = u.info.Defs[ident]
		}
		if s := u.suiteFor(obj); s != nil && s.migrated() && ident.Name != s.newName {
			ident.Name = s.newName
			changed = true
		}
		return true
	})

	// 2. Drop the runners gotest generates instead, and the assertions
	// that a suite implements testify's interfaces.
	decls := make([]ast.Decl, 0, len(f.Decls))
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && u.runnerSuite(fd) != nil {
			changed = true
			continue
		}
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			specs := gd.Specs[:0:0]
			for _, spec := range gd.Specs {
				if u.isSuiteContract(spec) {
					changed = true
					continue
				}
				specs = append(specs, spec)
			}
			if len(specs) == 0 {
				continue
			}
			gd.Specs = specs
		}
		decls = append(decls, decl)
	}
	f.Decls = decls

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			// 3. Drop the suite.Suite embedding.
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				s := u.suiteFor(u.info.Defs[ts.Name])
				st, ok := ts.Type.(*ast.StructType)
				if s == nil || !s.migrated() || !ok {
					continue
				}
				filtered := make([]*ast.Field, 0, len(st.Fields.List))
				for _, field := range st.Fields.List {
					if !u.isSuiteField(field) {
						filtered = append(filtered, field)
					}
				}
				st.Fields.List = filtered
				changed = true
			}

		case *ast.FuncDecl:
			changed = u.transformFunc(f, d) || changed
		}
	}

	if changed {
		u.rewriteImports(f, orig)
	}
	return changed
}

// transformFunc migrates one function: suite methods gotest calls gain t
// and take their gotest names, then assertions, s.T(), s.Run and calls of
// those methods are rewritten where a t is in scope.
func (u *unit) transformFunc(f *ast.File, d *ast.FuncDecl) bool {
	vars, onSuite := u.suiteVars(d)
	changed := false
	if onSuite {
		name := d.Name.Name
		newName, hook := lifecycleRenames[name]
		if hook || strings.HasPrefix(name, "Test") {
			if hook {
				d.Name.Name = newName
			}
			addGotestTParam(d, freeName(d))
			u.pointerReceiver(d)
			changed = true
		}
	}
	if d.Body == nil {
		return changed
	}
	tName, kind, tObj := tParam(d, u.info)
	r := &rewriter{u: u, f: f, tName: tName, tObj: tObj, gotestT: kind == "gotest"}
	if kind != "" {
		if r.gotestT && strings.HasPrefix(d.Name.Name, "Test") && onSuite {
			changed = r.subtests(d.Body, vars) || changed
		}
		for _, v := range vars {
			changed = r.assertions(d.Body, v) || changed
			if r.gotestT {
				changed = r.stCalls(d.Body, v) || changed
			}
		}
	}
	return r.methodCalls(d.Body) || changed
}

// isSuiteContract reports whether spec is `var _ suite.X = (*S)(nil)` for a
// migrated S: a testify interface the suite no longer implements.
func (u *unit) isSuiteContract(spec ast.Spec) bool {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok || vs.Type == nil || len(vs.Values) != len(vs.Names) {
		return false
	}
	sel, ok := vs.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if ident, ok := sel.X.(*ast.Ident); !ok || !u.isPkg(ident, suitePkg) {
		return false
	}
	for i, name := range vs.Names {
		if name.Name != "_" {
			return false
		}
		if s := u.suiteOfType(u.info.TypeOf(vs.Values[i])); s == nil || !s.migrated() {
			return false
		}
	}
	return true
}

// pointerReceiver gives a method gotest calls a pointer receiver, unless it
// changes the suite: testify ran it on a copy, so the change was dropped,
// and that is left for a marker.
func (u *unit) pointerReceiver(d *ast.FuncDecl) {
	recv := d.Recv.List[0]
	if _, ok := recv.Type.(*ast.StarExpr); ok {
		return
	}
	if len(recv.Names) > 0 && d.Body != nil && u.mutates(d.Body, recv.Names[0]) {
		return
	}
	recv.Type = &ast.StarExpr{X: recv.Type}
}

// mutates reports whether body may change the value recv names: assigns
// through it, takes its address or calls a pointer method on it.
func (u *unit) mutates(body *ast.BlockStmt, recv *ast.Ident) bool {
	obj := u.info.Defs[recv]
	if obj == nil {
		return false
	}
	rooted := func(e ast.Expr) bool {
		for {
			switch x := e.(type) {
			case *ast.SelectorExpr:
				if sel := u.info.Selections[x]; sel != nil && sel.Indirect() {
					return false // through a pointer field, shared either way
				}
				e = x.X
			case *ast.IndexExpr:
				if _, ok := u.info.TypeOf(x.X).Underlying().(*types.Array); !ok {
					return false
				}
				e = x.X
			case *ast.ParenExpr:
				e = x.X
			case *ast.Ident:
				return u.info.Uses[x] == obj
			default:
				return false
			}
		}
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE {
				for _, lhs := range x.Lhs {
					found = found || rooted(lhs)
				}
			}
		case *ast.IncDecStmt:
			found = found || rooted(x.X)
		case *ast.UnaryExpr:
			found = found || x.Op == token.AND && rooted(x.X)
		case *ast.SelectorExpr:
			if sel := u.info.Selections[x]; sel != nil && sel.Kind() == types.MethodVal && rooted(x.X) {
				if testifyObj(sel.Obj()) {
					return true // rewritten, not called
				}
				if sig, ok := sel.Obj().Type().(*types.Signature); ok && sig.Recv() != nil {
					if _, ptr := sig.Recv().Type().(*types.Pointer); ptr {
						found = true
					}
				}
			}
		}
		return !found
	})
	return found
}

// runnerSuite is the migrated suite fd does nothing but run: a Test func
// whose body is suite.Run(t, new(S)) or suite.Run(t, &S{}), perhaps after
// t.Parallel(). Any other runner stays, and its suite.Run is marked.
func (u *unit) runnerSuite(fd *ast.FuncDecl) *suiteDecl {
	if fd.Recv != nil || fd.Body == nil || !strings.HasPrefix(fd.Name.Name, "Test") {
		return nil
	}
	var run *ast.CallExpr
	for _, stmt := range fd.Body.List {
		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			return nil
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok {
			return nil
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Parallel" && len(call.Args) == 0 {
			continue
		}
		if run != nil || !u.isSuiteRun(call) {
			return nil
		}
		run = call
	}
	if run == nil {
		return nil
	}
	var typ ast.Expr
	switch a := run.Args[1].(type) {
	case *ast.CallExpr:
		if ident, ok := a.Fun.(*ast.Ident); ok && len(a.Args) == 1 {
			if b, ok := u.info.Uses[ident].(*types.Builtin); ok && b.Name() == "new" {
				typ = a.Args[0]
			}
		}
	case *ast.UnaryExpr:
		if lit, ok := a.X.(*ast.CompositeLit); ok && a.Op == token.AND && len(lit.Elts) == 0 {
			typ = lit.Type
		}
	}
	if s := u.suiteOf(typ); s != nil && s.migrated() {
		return s
	}
	return nil
}

// isSuiteRun reports whether call is testify's suite.Run(t, s).
func (u *unit) isSuiteRun(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" || len(call.Args) != 2 {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && u.isPkg(ident, suitePkg)
}

// suiteVars names fd's variables of a migrated suite type: its receiver
// (onSuite) and any parameter of one, as a free helper taking the suite has.
func (u *unit) suiteVars(fd *ast.FuncDecl) (vars []*ast.Ident, onSuite bool) {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		recv := fd.Recv.List[0]
		if s := u.suiteOf(recv.Type); s != nil && s.migrated() {
			onSuite = true
			if len(recv.Names) > 0 && recv.Names[0].Name != "_" {
				vars = append(vars, recv.Names[0])
			}
		}
	}
	if fd.Type.Params == nil {
		return vars, onSuite
	}
	for _, p := range fd.Type.Params.List {
		if s := u.suiteOf(p.Type); s == nil || !s.migrated() {
			continue
		}
		for _, name := range p.Names {
			if name.Name != "_" {
				vars = append(vars, name)
			}
		}
	}
	return vars, onSuite
}

// freeName is a name for t that nothing in fd spells: t, unless fd already
// uses it as a receiver, a local, a loop variable or anything else.
func freeName(fd *ast.FuncDecl) string {
	taken := map[string]bool{}
	ast.Inspect(fd, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			ast.Inspect(x.X, func(m ast.Node) bool {
				if ident, ok := m.(*ast.Ident); ok {
					taken[ident.Name] = true
				}
				return true
			})
			return false
		case *ast.Ident:
			taken[x.Name] = true
		}
		return true
	})
	for _, name := range []string{"t", "gt", "tt"} {
		if !taken[name] {
			return name
		}
	}
	for i := 1; ; i++ {
		if name := "t" + strconv.Itoa(i); !taken[name] {
			return name
		}
	}
}

// gotestTType is the *gotest.T type expression.
func gotestTType() ast.Expr {
	return &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("gotest"), Sel: ast.NewIdent("T")}}
}

// addGotestTParam adds `name *gotest.T` as the first parameter of fd.
func addGotestTParam(fd *ast.FuncDecl, name string) {
	tParam := &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: gotestTType()}
	if fd.Type.Params == nil {
		fd.Type.Params = &ast.FieldList{}
	}
	fd.Type.Params.List = append([]*ast.Field{tParam}, fd.Type.Params.List...)
}

// tParam finds fd's t: its first parameter of type *gotest.T ("gotest") or
// *testing.T ("testing"). obj is nil for a t the migration added. Both take
// a gotest assertion; only *gotest.T has the T() an s.T() becomes.
func tParam(fd *ast.FuncDecl, info *types.Info) (name, kind string, obj types.Object) {
	if fd.Type.Params == nil {
		return "", "", nil
	}
	for _, p := range fd.Type.Params.List {
		if len(p.Names) == 0 || p.Names[0].Name == "_" {
			continue
		}
		star, ok := p.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "T" {
			continue
		}
		if ident, ok := sel.X.(*ast.Ident); ok && (ident.Name == "gotest" || ident.Name == "testing") {
			return p.Names[0].Name, ident.Name, info.Defs[p.Names[0]]
		}
	}
	return "", "", nil
}

// rewriter rewrites one function body against its t.
type rewriter struct {
	u       *unit
	f       *ast.File
	tName   string
	tObj    types.Object // nil for a t the migration added
	gotestT bool
}

// reaches reports whether t, spelled tName, is the t in scope at pos: a
// t the function had may be shadowed there; one the migration added has
// a name nothing in the function spells.
func (r *rewriter) reaches(pos token.Pos) bool {
	return r.tObj == nil || r.u.lookup(r.f, r.tName, pos) == r.tObj
}

func (r *rewriter) tIdent() *ast.Ident { return ast.NewIdent(r.tName) }

// subtests turns `s.Run(name, func() {...})` statements into
// `t.It(name, func(t *gotest.T) {...})`: the closure's own t, under the same
// name, is what the assertions inside it then name.
func (r *rewriter) subtests(body *ast.BlockStmt, vars []*ast.Ident) bool {
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		es, ok := n.(*ast.ExprStmt)
		if !ok {
			return true
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" || !r.isSuiteVar(sel.X, vars) {
			return true
		}
		if s := r.u.info.Selections[sel]; s == nil || !testifyObj(s.Obj()) {
			return true // the suite's own Run
		}
		lit, ok := call.Args[1].(*ast.FuncLit)
		if !ok || lit.Type.Params.NumFields() != 0 || lit.Type.Results.NumFields() != 0 {
			return true
		}
		sel.X, sel.Sel = r.tIdent(), ast.NewIdent("It")
		lit.Type.Params.List = []*ast.Field{{Names: []*ast.Ident{r.tIdent()}, Type: gotestTType()}}
		changed = true
		return true
	})
	return changed
}

// isSuiteVar reports whether expr is one of vars.
func (r *rewriter) isSuiteVar(expr ast.Expr, vars []*ast.Ident) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	for _, v := range vars {
		if obj := r.u.info.Uses[ident]; obj != nil && obj == r.u.info.Defs[v] {
			return true
		}
	}
	return false
}

// assertions rewrites the testify assertion statements in body, nested
// blocks included, and reports whether it rewrote any.
func (r *rewriter) assertions(body *ast.BlockStmt, recv *ast.Ident) bool {
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		es, ok := n.(*ast.ExprStmt)
		if !ok {
			return true
		}
		if call, ok := es.X.(*ast.CallExpr); ok && r.reaches(call.Pos()) {
			if newExpr := r.assertionCall(call, recv); newExpr != nil {
				es.X = newExpr
				changed = true
			}
		}
		return true
	})
	return changed
}

// assertionCall rewrites one assertion call, nil when no rewrite applies:
// s.Require().X(...), s.Assert().X(...), the suite's own s.X(...), and
// assert.X(s.T(), ...) or require.X(s.T(), ...).
func (r *rewriter) assertionCall(call *ast.CallExpr, recv *ast.Ident) ast.Expr {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	args := call.Args
	switch x := sel.X.(type) {
	case *ast.CallExpr:
		inner, ok := x.Fun.(*ast.SelectorExpr)
		if !ok || !r.isSuiteVar(inner.X, []*ast.Ident{recv}) || (inner.Sel.Name != "Require" && inner.Sel.Name != "Assert") {
			return nil
		}
	case *ast.Ident:
		switch {
		case r.u.isPkg(x, assertPkg, requirePkg):
			if len(args) == 0 || r.stCallReceiver(args[0], recv) == nil {
				return nil
			}
			args = args[1:]
		case r.isSuiteVar(x, []*ast.Ident{recv}):
			// The suite's own assertion methods, promoted from the
			// *assert.Assertions suite.Suite embeds.
			s := r.u.info.Selections[sel]
			if s == nil || s.Obj().Pkg() == nil || s.Obj().Pkg().Path() != assertPkg {
				return nil
			}
		default:
			return nil
		}
	default:
		return nil
	}
	name, newArgs, _ := r.u.mapAssertion(r.f, sel.Sel.Name, args)
	if name == "" {
		return nil
	}
	fun, err := parser.ParseExpr("gotest." + name)
	if err != nil {
		return nil
	}
	return &ast.CallExpr{Fun: fun, Args: append([]ast.Expr{r.tIdent()}, newArgs...)}
}

// stCalls rewrites every s.T() in body to t.T() and reports whether it
// found any. Only a function with a *gotest.T may call it; elsewhere s.T()
// is left for a marker.
func (r *rewriter) stCalls(body *ast.BlockStmt, recv *ast.Ident) bool {
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		if ident := r.stCallReceiver(n, recv); ident != nil && r.reaches(ident.Pos()) {
			ident.Name = r.tName
			changed = true
		}
		return true
	})
	return changed
}

// stCallReceiver returns the receiver identifier of an s.T() call, nil for
// any other node.
func (r *rewriter) stCallReceiver(n ast.Node, recv *ast.Ident) *ast.Ident {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return nil
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || !r.isSuiteVar(ident, []*ast.Ident{recv}) {
		return nil
	}
	return ident
}

// methodCalls follows the suite methods that gained t: a call takes the
// method's gotest name and passes t along; a method value is only renamed
// and left for a marker.
func (r *rewriter) methodCalls(body *ast.BlockStmt) bool {
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, ok := r.hookOf(sel); ok && r.gotestT && r.reaches(call.Pos()) {
			call.Args = append([]ast.Expr{r.tIdent()}, call.Args...)
			changed = true
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if newName, ok := r.hookOf(sel); ok && sel.Sel.Name != newName {
				sel.Sel.Name = newName
				changed = true
			}
		}
		return true
	})
	return changed
}

// hookOf is the gotest name of the suite method sel selects, when the
// migration gives that method t.
func (r *rewriter) hookOf(sel *ast.SelectorExpr) (string, bool) {
	s := r.u.info.Selections[sel]
	if s == nil || s.Kind() != types.MethodVal {
		return "", false
	}
	fn, ok := s.Obj().(*types.Func)
	if !ok {
		return "", false
	}
	name, ok := r.u.hooks[methodKey(fn)]
	return name, ok
}

// rewriteImports removes the testify and testing imports nothing in the
// migrated f refers to any more and adds the gotest import once, if
// anything uses it. A use is an identifier that resolves to the import: a
// local variable named suite is not the package. A dot import stays while
// any name the unit does not declare is left for it to provide.
func (u *unit) rewriteImports(f *ast.File, orig map[*ast.Ident]bool) {
	removable := map[string]bool{
		suitePkg:   true,
		assertPkg:  true,
		requirePkg: true,
		"testing":  true,
	}

	used := map[types.Object]bool{}
	skip := map[*ast.Ident]bool{f.Name: true}
	needGotest, dotUsed := false, false
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			skip[x.Sel] = true
			if ident, ok := x.X.(*ast.Ident); ok && ident.Name == "gotest" && !orig[ident] {
				needGotest = true
			}
		case *ast.KeyValueExpr:
			// A field key of a struct whose type did not resolve.
			if ident, ok := x.Key.(*ast.Ident); ok {
				skip[ident] = true
			}
		case *ast.ImportSpec:
			if x.Name != nil {
				skip[x.Name] = true
			}
		case *ast.Ident:
			if obj := u.info.Uses[x]; obj != nil {
				used[obj] = true
				return true
			}
			if _, ok := u.info.Defs[x]; ok || !orig[x] || skip[x] || x.Name == "_" {
				return true
			}
			dotUsed = true
		}
		return true
	})

	hasGotest := false
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		filtered := make([]ast.Spec, 0, len(gd.Specs))
		for _, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok {
				filtered = append(filtered, spec)
				continue
			}
			path := strings.Trim(is.Path.Value, `"`)
			if path == gotestImport {
				hasGotest = true
			}
			if removable[path] && !u.importUsed(is, used, dotUsed) {
				continue
			}
			filtered = append(filtered, spec)
		}
		gd.Specs = filtered
	}

	// An import block left empty goes.
	decls := f.Decls[:0]
	for _, decl := range f.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.IMPORT && len(gd.Specs) == 0 {
			continue
		}
		decls = append(decls, decl)
	}
	f.Decls = decls

	if !hasGotest && needGotest {
		addImport(f, gotestImport)
	}
}

// addImport appends path to f's first import block, or opens one; the
// formatting pass then groups it the way goimports does.
func addImport(f *ast.File, path string) {
	spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", path)}}
	for _, decl := range f.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			if !gd.Lparen.IsValid() {
				gd.Lparen = gd.Specs[0].Pos()
				gd.Rparen = gd.Specs[0].End()
			}
			gd.Specs = append(gd.Specs, spec)
			return
		}
	}
	f.Decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{spec}}}, f.Decls...)
}

// importUsed reports whether the import is still referred to.
func (u *unit) importUsed(is *ast.ImportSpec, used map[types.Object]bool, dotUsed bool) bool {
	if is.Name != nil {
		switch is.Name.Name {
		case "_":
			return true
		case ".":
			path, _ := strconv.Unquote(is.Path.Value)
			for obj := range used {
				if obj.Pkg() != nil && obj.Pkg().Path() == path && obj.Parent() == obj.Pkg().Scope() {
					return true
				}
			}
			return dotUsed
		}
		return used[u.info.Defs[is.Name]]
	}
	return used[u.info.Implicits[is]]
}

// testifyObj reports whether obj is declared by a testify package.
func testifyObj(obj types.Object) bool {
	return obj.Pkg() != nil && strings.HasPrefix(obj.Pkg().Path(), "github.com/stretchr/testify/")
}
