package lint

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// softBase returns X when e is X.Soft() on a *gotest.T, and e otherwise. A
// variable holding a soft T is not followed: it reads as fatal.
func softBase(pass *analysis.Pass, e ast.Expr) ast.Expr {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return e
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Soft" || !namedPtrType(pass.TypesInfo.TypeOf(sel.X), gotestImportPath, "T") {
		return e
	}
	return sel.X
}

// isSoft reports whether e is X.Soft() on a *gotest.T: an assertion given
// it goes on after failing.
func isSoft(pass *analysis.Pass, e ast.Expr) bool {
	return softBase(pass, e) != e
}
