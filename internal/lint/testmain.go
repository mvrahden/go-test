package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
)

var runtimeImportPath = about.Repo + "/pkg/gotestruntime"

// checkTestMainFixtureTeardown flags a TestMain that runs the tests without
// gotestruntime in a package whose suites bind fixtures. Fixtures tear down
// when gotestruntime.Main or gotestruntime.M finishes; fixture setup refuses to
// start otherwise, so this reports at the source what the run would fail on.
// The fix replaces m.Run() with gotestruntime.Main(m) and wraps m where it is
// handed to a library that takes it as an interface{ Run() int }, and imports
// the runtime when the file does not.
//
// It reads the TestMain alone: a helper that routes the tests through the
// runtime is not seen, and is suppressed per line.
func checkTestMainFixtureTeardown(pass *analysis.Pass, suites map[string]*suiteInfo) {
	binds := false
	for _, s := range suites {
		if len(s.fixtureFields) > 0 {
			binds = true
			break
		}
	}
	if !binds {
		return
	}
	fd, file := gotestast.FindTestMain(pass.Fset, pass.Files)
	if fd == nil || gotestast.RoutesThroughRuntime(file, fd) {
		return
	}
	reportWithFix(pass, TestMainFixtureTeardown, fd.Name.Pos(), testMainFix(pass, file, fd),
		"TestMain must run the tests through gotestruntime so fixtures tear down after them: replace m.Run() with %s, or pass %s to a library that takes m",
		gotestast.RuntimeMain, gotestast.RuntimeM)
}

// testMainFix rewrites every m.Run() in fd, where m is its *testing.M, and
// wraps every m passed where an interface{ Run() int } is expected. It offers
// nothing when there is neither.
func testMainFix(pass *analysis.Pass, file *ast.File, fd *ast.FuncDecl) []analysis.SuggestedFix {
	params := fd.Type.Params.List
	if len(params) != 1 || len(params[0].Names) != 1 || fd.Body == nil {
		return nil
	}
	param := params[0].Names[0]
	isM := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && pass.TypesInfo.Uses[id] != nil && pass.TypesInfo.Uses[id] == pass.TypesInfo.Defs[param]
	}
	qual, imported := runtimeQualifier(file)

	// A TestMain that returns makes Go exit with m.Run's own code, which knows
	// nothing of the teardown: a discarded m.Run() that ends TestMain becomes
	// os.Exit(gotestruntime.Main(m)). One elsewhere has no rewrite that keeps
	// the control flow, so it is reported without one.
	discarded := map[*ast.CallExpr]bool{}
	var exitEdit *analysis.TextEdit
	needOS := false
	for i, stmt := range fd.Body.List {
		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok || !isRunOf(call, isM) {
			continue
		}
		discarded[call] = true
		if i == len(fd.Body.List)-1 {
			osq, hasOS := importQualifier(file, "os", "os.")
			needOS = !hasOS
			exitEdit = &analysis.TextEdit{Pos: es.Pos(), End: es.End(), NewText: []byte(osq + "Exit(" + qual + "Main(" + param.Name + "))")}
		}
	}

	var edits []analysis.TextEdit
	if exitEdit != nil {
		edits = append(edits, *exitEdit)
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isRunOf(call, isM) {
			if !discarded[call] {
				edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.End(), NewText: []byte(qual + "Main(" + param.Name + ")")})
			}
			return true
		}
		sig, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
		if !ok {
			return true
		}
		for i, arg := range call.Args {
			if isM(arg) && takesRunner(sig, i) {
				edits = append(edits, analysis.TextEdit{Pos: arg.Pos(), End: arg.End(), NewText: []byte(qual + "M(" + param.Name + ")")})
			}
		}
		return true
	})
	if len(edits) == 0 {
		return nil
	}
	var paths []string
	if !imported {
		paths = append(paths, runtimeImportPath)
	}
	if needOS {
		paths = append(paths, "os")
	}
	if len(paths) > 0 {
		edits = append(edits, importEdit(file, paths...))
	}
	return []analysis.SuggestedFix{{Message: "run the tests through gotestruntime", TextEdits: edits}}
}

// takesRunner reports whether parameter i of sig is an interface whose only
// method is Run() int, the shape goleak and testscript take m as.
func takesRunner(sig *types.Signature, i int) bool {
	params := sig.Params()
	if params.Len() == 0 {
		return false
	}
	if i >= params.Len() {
		if !sig.Variadic() {
			return false
		}
		i = params.Len() - 1
	}
	t := params.At(i).Type()
	if sig.Variadic() && i == params.Len()-1 {
		if s, ok := t.(*types.Slice); ok {
			t = s.Elem()
		}
	}
	iface, ok := t.Underlying().(*types.Interface)
	if !ok || iface.NumMethods() != 1 || iface.Method(0).Name() != "Run" {
		return false
	}
	run, ok := iface.Method(0).Type().(*types.Signature)
	if !ok || run.Params().Len() != 0 || run.Results().Len() != 1 {
		return false
	}
	basic, ok := run.Results().At(0).Type().(*types.Basic)
	return ok && basic.Kind() == types.Int
}

// runtimeQualifier returns how file names the runtime package, and whether it
// imports it at all.
func runtimeQualifier(file *ast.File) (qual string, imported bool) {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != runtimeImportPath {
			continue
		}
		if imp.Name != nil {
			switch imp.Name.Name {
			case ".":
				return "", true
			case "_":
				continue
			}
			return imp.Name.Name + ".", true
		}
		return "gotestruntime.", true
	}
	return "gotestruntime.", false
}

// importQualifier returns how file names the package at path, or fallback and
// false when it does not import it.
func importQualifier(file *ast.File, path, fallback string) (string, bool) {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != path {
			continue
		}
		if imp.Name == nil {
			return fallback, true
		}
		switch imp.Name.Name {
		case ".":
			return "", true
		case "_":
			continue
		}
		return imp.Name.Name + ".", true
	}
	return fallback, false
}

// isRunOf reports whether call is m.Run() on the TestMain's m.
func isRunOf(call *ast.CallExpr, isM func(ast.Expr) bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Run" && len(call.Args) == 0 && isM(sel.X)
}

// importEdit adds paths to file's imports, in one edit: into the first
// parenthesized import block, else as a declaration of its own after the
// package clause.
func importEdit(file *ast.File, paths ...string) analysis.TextEdit {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		if gd.Lparen.IsValid() {
			var b strings.Builder
			for _, p := range paths {
				b.WriteString("\t\"" + p + "\"\n")
			}
			return analysis.TextEdit{Pos: gd.Rparen, End: gd.Rparen, NewText: []byte(b.String())}
		}
	}
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("\n\nimport \"" + p + "\"")
	}
	return analysis.TextEdit{Pos: file.Name.End(), End: file.Name.End(), NewText: []byte(b.String())}
}
