package lint

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

func checkRedundantAssertion(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.BlockStmt)(nil)}, func(n ast.Node) {
		block := n.(*ast.BlockStmt)
		for i := 0; i < len(block.List)-1; i++ {
			first := extractAssertionCall(pass, block.List[i])
			second := extractAssertionCall(pass, block.List[i+1])
			if first == nil || second == nil {
				continue
			}
			if !sameExprText(pass, first.guardedArg, second.guardedArg) {
				continue
			}
			reason := isRedundantBefore(first.name, second.name, second.call)
			if reason == "" {
				continue
			}
			reportWithFix(pass, AssertionRedundant, first.call.Pos(),
				[]analysis.SuggestedFix{{
					Message: "remove redundant assertion",
					TextEdits: []analysis.TextEdit{{
						Pos:     block.List[i].Pos(),
						End:     block.List[i+1].Pos(),
						NewText: []byte(""),
					}},
				}},
				"%s is redundant before %s — %s",
				first.name, second.name, reason)
		}
	})
}

type parsedAssertion struct {
	name       string
	call       *ast.CallExpr
	guardedArg ast.Expr
}

func extractAssertionCall(pass *analysis.Pass, stmt ast.Stmt) *parsedAssertion {
	es, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok {
		return nil
	}
	name := assertionFuncName(pass, call.Fun)
	if name == "" || len(call.Args) < 2 {
		return nil
	}
	return &parsedAssertion{name: name, call: call, guardedArg: call.Args[1]}
}

func sameExprText(pass *analysis.Pass, a, b ast.Expr) bool {
	return renderExpr(pass.Fset, a) == renderExpr(pass.Fset, b)
}

func isRedundantBefore(weaker, stronger string, strongerCall *ast.CallExpr) string {
	switch {
	case weaker == "Error" && stronger == "ErrorIs":
		return "ErrorIs already checks for non-nil error"
	case weaker == "Error" && stronger == "ErrorContains":
		return "ErrorContains already checks for non-nil error"
	case weaker == "Error" && stronger == "ErrorAs":
		return "ErrorAs already checks for non-nil error"

	case weaker == "NotNil" && stronger == "NotEmpty":
		return "NotEmpty already checks for nil"
	case weaker == "NotNil" && stronger == "Len":
		if isNonZeroIntLit(strongerCall) {
			return "Len already handles nil for non-zero lengths"
		}
	case weaker == "NotNil" && stronger == "Contains":
		return "Contains already handles nil"

	case weaker == "NotEmpty" && stronger == "Len":
		if isNonZeroIntLit(strongerCall) {
			return "Len already implies non-empty for non-zero lengths"
		}
	case weaker == "NotEmpty" && stronger == "Contains":
		return "Contains already implies non-empty"
	}
	return ""
}

func isNonZeroIntLit(lenCall *ast.CallExpr) bool {
	if len(lenCall.Args) < 3 {
		return false
	}
	lit, ok := lenCall.Args[2].(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value != "0"
}
