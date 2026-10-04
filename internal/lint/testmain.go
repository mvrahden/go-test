package lint

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
)

var runtimeImportPath = about.Repo + "/pkg/gotestruntime"

// checkTestMainFixtureTeardown flags a TestMain that runs the tests with m.Run
// in a package whose suites bind fixtures. Fixtures tear down after the tests
// inside gotestruntime.Main; a TestMain that skips it leaves them up until the
// process exits, and generation refuses the package. The fix replaces each
// m.Run() with gotestruntime.Main(m) and imports the runtime when the file
// does not.
//
// A pass sees one variant of the test binary, so a TestMain in the external
// test package over suites in the internal one is left to generation.
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
	if fd == nil || gotestast.CallsRuntimeMain(file, fd) {
		return
	}
	reportWithFix(pass, TestMainFixtureTeardown, fd.Name.Pos(), testMainFix(pass, file, fd),
		"TestMain must call %s so fixtures tear down after the tests: replace m.Run() with %s",
		gotestast.RuntimeMain, gotestast.RuntimeMain)
}

// testMainFix rewrites every m.Run() in fd, where m is its *testing.M. It
// offers nothing when there is no such call to rewrite.
func testMainFix(pass *analysis.Pass, file *ast.File, fd *ast.FuncDecl) []analysis.SuggestedFix {
	params := fd.Type.Params.List
	if len(params) != 1 || len(params[0].Names) != 1 || fd.Body == nil {
		return nil
	}
	m := params[0].Names[0].Name
	qual, imported := runtimeQualifier(file)

	var edits []analysis.TextEdit
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == m && pass.TypesInfo.Uses[id] == pass.TypesInfo.Defs[params[0].Names[0]] {
			edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.End(), NewText: []byte(qual + "Main(" + m + ")")})
		}
		return true
	})
	if len(edits) == 0 {
		return nil
	}
	if !imported {
		edits = append(edits, importEdit(file, runtimeImportPath))
	}
	return []analysis.SuggestedFix{{Message: "call " + gotestast.RuntimeMain, TextEdits: edits}}
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
