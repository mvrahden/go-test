package migrate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
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
		if s := u.byObj[obj]; s != nil && s.migrated() && ident.Name != s.newName {
			ident.Name = s.newName
			changed = true
		}
		return true
	})

	// 2. Drop the runners gotest generates instead.
	decls := make([]ast.Decl, 0, len(f.Decls))
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && u.runnerSuite(fd) != nil {
			changed = true
			continue
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
				s := u.byObj[u.info.Defs[ts.Name]]
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
			// 4. Methods gain t; hooks take their gotest names.
			vars, onSuite := u.suiteVars(d)
			if len(vars) == 0 {
				continue
			}
			if onSuite {
				if newName, ok := lifecycleRenames[d.Name.Name]; ok {
					d.Name.Name = newName
					addGotestTParam(d)
					changed = true
				} else if strings.HasPrefix(d.Name.Name, "Test") {
					addGotestTParam(d)
					changed = true
				}
			}

			// 5. Rewrite assertions only where t is in scope, and s.T()
			// only where t is a *gotest.T; the rest is left for a marker.
			kind := tParamKind(d)
			if d.Body == nil || kind == "" {
				continue
			}
			for _, v := range vars {
				changed = u.rewriteAssertions(d.Body, v) || changed
				if kind == "gotest" {
					changed = rewriteSTCalls(d.Body, v) || changed
				}
			}
		}
	}

	if changed {
		u.rewriteImports(f, orig)
	}
	return changed
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
func (u *unit) suiteVars(fd *ast.FuncDecl) (vars []string, onSuite bool) {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		recv := fd.Recv.List[0]
		if s := u.suiteOf(recv.Type); s != nil && s.migrated() {
			onSuite = true
			if len(recv.Names) > 0 && recv.Names[0].Name != "_" {
				vars = append(vars, recv.Names[0].Name)
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
				vars = append(vars, name.Name)
			}
		}
	}
	return vars, onSuite
}

// addGotestTParam adds `t *gotest.T` as the first parameter of a function.
func addGotestTParam(fd *ast.FuncDecl) {
	tParam := &ast.Field{
		Names: []*ast.Ident{ast.NewIdent("t")},
		Type: &ast.StarExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent("gotest"),
				Sel: ast.NewIdent("T"),
			},
		},
	}
	if fd.Type.Params == nil {
		fd.Type.Params = &ast.FieldList{}
	}
	fd.Type.Params.List = append([]*ast.Field{tParam}, fd.Type.Params.List...)
}

// rewriteAssertions rewrites the testify assertion statements in body,
// nested blocks included, and reports whether it rewrote any.
func (u *unit) rewriteAssertions(body *ast.BlockStmt, recvName string) bool {
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		es, ok := n.(*ast.ExprStmt)
		if !ok {
			return true
		}
		if call, ok := es.X.(*ast.CallExpr); ok {
			if newExpr := u.rewriteAssertionCall(call, recvName); newExpr != nil {
				es.X = newExpr
				changed = true
			}
		}
		return true
	})
	return changed
}

// rewriteAssertionCall attempts to rewrite a single assertion call.
// Returns nil if no rewrite is applicable.
func (u *unit) rewriteAssertionCall(call *ast.CallExpr, recvName string) ast.Expr {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	gotestFunc, mapped := assertionMap[sel.Sel.Name]
	if !mapped {
		return nil
	}

	// Pattern 1: s.Require().Method(args...) or s.Assert().Method(args...)
	if innerCall, ok := sel.X.(*ast.CallExpr); ok {
		if innerSel, ok := innerCall.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := innerSel.X.(*ast.Ident); ok && ident.Name == recvName {
				if innerSel.Sel.Name == "Require" || innerSel.Sel.Name == "Assert" {
					return makeGotestCall(gotestFunc, call.Args)
				}
			}
		}
	}

	// Pattern 2: assert.Method(s.T(), args...) or require.Method(s.T(), args...)
	if pkgIdent, ok := sel.X.(*ast.Ident); ok && u.isPkg(pkgIdent, assertPkg, requirePkg) {
		if len(call.Args) >= 1 && stCallReceiver(call.Args[0], recvName) != nil {
			return makeGotestCall(gotestFunc, call.Args[1:])
		}
	}
	return nil
}

// makeGotestCall creates a gotest.Func(t, args...) call expression.
func makeGotestCall(funcName string, args []ast.Expr) *ast.CallExpr {
	newArgs := make([]ast.Expr, 0, len(args)+1)
	newArgs = append(newArgs, ast.NewIdent("t"))
	newArgs = append(newArgs, args...)

	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("gotest"),
			Sel: ast.NewIdent(funcName),
		},
		Args: newArgs,
	}
}

// rewriteSTCalls rewrites every s.T() in body to t.T() and reports whether
// it found any. Only a method that gained the t parameter may call it;
// elsewhere s.T() is left for a marker.
func rewriteSTCalls(body *ast.BlockStmt, recvName string) bool {
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		if ident := stCallReceiver(n, recvName); ident != nil {
			ident.Name = "t"
			changed = true
		}
		return true
	})
	return changed
}

// stCallReceiver returns the receiver identifier of an s.T() call, nil for
// any other node.
func stCallReceiver(n ast.Node, recvName string) *ast.Ident {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return nil
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != recvName {
		return nil
	}
	return ident
}

// tParamKind reports which t fd declares: "gotest" for t *gotest.T,
// "testing" for t *testing.T, "" for none. Both take a gotest assertion; only
// *gotest.T has the T() an s.T() becomes.
func tParamKind(fd *ast.FuncDecl) string {
	if fd.Type.Params == nil {
		return ""
	}
	for _, p := range fd.Type.Params.List {
		if len(p.Names) == 0 || p.Names[0].Name != "t" {
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
			return ident.Name
		}
	}
	return ""
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

	var first *ast.GenDecl
	hasGotest := false
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		if first == nil {
			first = gd
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

	if !hasGotest && needGotest {
		spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", gotestImport)}}
		if first == nil {
			first = &ast.GenDecl{Tok: token.IMPORT}
			f.Decls = append([]ast.Decl{first}, f.Decls...)
		}
		first.Specs = append(first.Specs, spec)
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
}

// importUsed reports whether the import is still referred to.
func (u *unit) importUsed(is *ast.ImportSpec, used map[types.Object]bool, dotUsed bool) bool {
	if is.Name != nil {
		switch is.Name.Name {
		case "_":
			return true
		case ".":
			return dotUsed
		}
		return used[u.info.Defs[is.Name]]
	}
	return used[u.info.Implicits[is]]
}
