package lint

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

func checkRedundantAssertion(pass *analysis.Pass, insp *inspector.Inspector, cl *claims) {
	insp.Preorder([]ast.Node{(*ast.BlockStmt)(nil)}, func(n ast.Node) {
		block := n.(*ast.BlockStmt)
		for i := 0; i < len(block.List)-1; i++ {
			first := extractAssertionCall(pass, block.List[i])
			second := extractAssertionCall(pass, block.List[i+1])
			if first == nil || second == nil {
				continue
			}
			if cl.anyWithin(first.call.Pos(), first.call.End()) || cl.anyWithin(second.call.Pos(), second.call.End()) {
				continue
			}
			if !sameExprText(pass, first.guardedArg, second.guardedArg) {
				continue
			}
			reason := isRedundantBefore(pass, first, second)
			if reason == "" {
				continue
			}
			reportWithFix(pass, AssertionRedundant, first.call.Pos(),
				redundantFix(pass, block.List[i], block.List[i+1], first, second),
				"%s is redundant before %s — %s",
				first.name, second.name, reason)
		}
	})
}

// redundantFix removes the weaker assertion. Its message moves to the
// stronger one, and its comments stay above it; when both carry a message
// there is no fix.
func redundantFix(pass *analysis.Pass, stmt, next ast.Stmt, weaker, stronger *parsedAssertion) []analysis.SuggestedFix {
	msgs := weaker.call.Args[2:]
	if len(msgs) > 0 && len(stronger.call.Args) > messageIndex(stronger.name) {
		return nil
	}
	src, ok := sourceAt(pass, stmt.Pos())
	if !ok {
		return nil
	}
	indent, ok := src.lineIndent(next.Pos())
	if !ok {
		return nil
	}
	edits := []analysis.TextEdit{{Pos: stmt.Pos(), End: next.Pos()}}
	if stray := src.strayComments(stmt.Pos(), next.Pos(), msgs); len(stray) > 0 {
		edits[0].NewText = []byte(strings.Join(stray, "\n"+indent) + "\n" + indent)
	}
	if len(msgs) > 0 {
		parts := make([]string, len(msgs))
		for i, m := range msgs {
			parts[i] = src.text(pass.Fset, m)
		}
		last := stronger.call.Args[len(stronger.call.Args)-1]
		edits = append(edits, analysis.TextEdit{Pos: last.End(), End: last.End(), NewText: []byte(", " + strings.Join(parts, ", "))})
	}
	return []analysis.SuggestedFix{{Message: "remove redundant assertion", TextEdits: edits}}
}

// messageIndex is where the stronger assertion's msgAndArgs begin.
func messageIndex(name string) int {
	switch name {
	case "ErrorAs", "NotEmpty":
		return 2
	}
	return 3
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

// isRedundantBefore says why first may go: the second fails on every input
// the first fails. NotNil fails its type guard on what cannot be nil, and
// Contains finds an empty string in an empty one.
func isRedundantBefore(pass *analysis.Pass, first, second *parsedAssertion) string {
	weaker, stronger, strongerCall := first.name, second.name, second.call
	if weaker == "NotNil" && nilKindOf(pass, first.guardedArg) == notNilable {
		return ""
	}
	if weaker == "NotEmpty" && stronger == "Contains" && mayBeEmptyString(pass, strongerCall.Args[2]) {
		return ""
	}
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
