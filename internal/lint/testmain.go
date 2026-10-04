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
	fd, file := gotestast.FindTestMain(pass.Files)
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

	var edits []analysis.TextEdit
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" && len(call.Args) == 0 && isM(sel.X) {
			edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.End(), NewText: []byte(qual + "Main(" + param.Name + ")")})
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
	if !imported {
		edits = append(edits, importEdit(file, runtimeImportPath))
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
			if imp.Name.Name == "." {
				return "", true
			}
			return imp.Name.Name + ".", true
		}
		return "gotestruntime.", true
	}
	return "gotestruntime.", false
}

// importEdit adds path to file's imports: into the first parenthesized import
// block, else as a declaration of its own after the package clause.
func importEdit(file *ast.File, path string) analysis.TextEdit {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		if gd.Lparen.IsValid() {
			return analysis.TextEdit{Pos: gd.Rparen, End: gd.Rparen, NewText: []byte("\t\"" + path + "\"\n")}
		}
	}
	return analysis.TextEdit{Pos: file.Name.End(), End: file.Name.End(), NewText: []byte("\n\nimport \"" + path + "\"")}
}
