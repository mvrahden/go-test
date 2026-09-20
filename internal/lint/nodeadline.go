package lint

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// deadlineFields maps each config type to the durations a negative value
// disables.
var deadlineFields = map[string][]string{
	"SuiteConfig":   {"Timeout", "SetupTimeout"},
	"FixtureConfig": {"Timeout"},
}

// checkConfigNoDeadline flags a literal negative duration on a config timeout,
// in a keyed literal or a field assignment. Every negative disables the
// deadline, so the fix spells it gotest.NoDeadline: a suite that runs unbounded
// then says so in a word, and the name is what a reader — or a grep over the
// repository — finds. Runtime values are not reported.
func checkConfigNoDeadline(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.CompositeLit)(nil), (*ast.AssignStmt)(nil)}, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.CompositeLit:
			typeName, ok := configTypeName(pass.TypesInfo.TypeOf(n))
			if !ok {
				return
			}
			for _, elt := range n.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok && isDeadlineField(typeName, key.Name) {
					reportNegativeTimeout(pass, typeName, key.Name, kv.Value)
				}
			}
		case *ast.AssignStmt:
			if n.Tok != token.ASSIGN || len(n.Lhs) != len(n.Rhs) {
				return
			}
			for i, lhs := range n.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				typeName, ok := configTypeName(pass.TypesInfo.TypeOf(sel.X))
				if ok && isDeadlineField(typeName, sel.Sel.Name) {
					reportNegativeTimeout(pass, typeName, sel.Sel.Name, n.Rhs[i])
				}
			}
		}
	})
}

// configTypeName names the gotest config type t is, or points to.
func configTypeName(t types.Type) (string, bool) {
	if t == nil {
		return "", false
	}
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		t = ptr.Elem()
	}
	for name := range deadlineFields {
		if namedType(t, gotestImportPath, name) {
			return name, true
		}
	}
	return "", false
}

func isDeadlineField(typeName, field string) bool {
	for _, f := range deadlineFields[typeName] {
		if f == field {
			return true
		}
	}
	return false
}

// reportNegativeTimeout reports value when it is a constant negative that is
// not NoDeadline itself. The fix is declined when the file cannot name the
// gotest package.
func reportNegativeTimeout(pass *analysis.Pass, typeName, field string, value ast.Expr) {
	tv, ok := pass.TypesInfo.Types[value]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int || constant.Sign(tv.Value) >= 0 {
		return
	}
	if isNoDeadline(pass, value) {
		return
	}
	var fixes []analysis.SuggestedFix
	if qual, ok := gotestQualifier(pass, value.Pos()); ok {
		fixes = []analysis.SuggestedFix{{
			Message: "write NoDeadline",
			TextEdits: []analysis.TextEdit{{
				Pos:     value.Pos(),
				End:     value.End(),
				NewText: []byte(qual + "NoDeadline"),
			}},
		}}
	}
	reportWithFix(pass, ConfigNoDeadline, value.Pos(), fixes,
		"%s.%s is negative, which disables the deadline; write gotest.NoDeadline to say so",
		typeName, field)
}

// isNoDeadline reports whether expr already names gotest.NoDeadline, whose own
// value is negative.
func isNoDeadline(pass *analysis.Pass, expr ast.Expr) bool {
	var ident *ast.Ident
	switch e := expr.(type) {
	case *ast.Ident:
		ident = e
	case *ast.SelectorExpr:
		ident = e.Sel
	default:
		return false
	}
	obj, ok := pass.TypesInfo.Uses[ident].(*types.Const)
	return ok && obj.Name() == "NoDeadline" && obj.Pkg() != nil && obj.Pkg().Path() == gotestImportPath
}

// namedType reports whether t is pkgPath.name, looking through aliases.
func namedType(t types.Type, pkgPath, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == name && obj.Pkg() != nil && obj.Pkg().Path() == pkgPath
}
