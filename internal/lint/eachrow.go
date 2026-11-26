package lint

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// eachRowVars is the variables holding an Each row's T: the first range
// variable of a `range gotest.Each(...)` loop and its plain aliases. A
// failing assertion on that T fails the row and goes on.
func eachRowVars(pass *analysis.Pass, insp *inspector.Inspector) map[types.Object]bool {
	rows := map[types.Object]bool{}
	insp.Preorder([]ast.Node{(*ast.RangeStmt)(nil)}, func(n ast.Node) {
		rs := n.(*ast.RangeStmt)
		if key, ok := rs.Key.(*ast.Ident); ok && isEachCall(pass, rs.X) {
			if obj := pass.TypesInfo.ObjectOf(key); obj != nil {
				rows[obj] = true
			}
		}
	})
	// r := row, var r = row and r = row; again until no alias of an alias is new.
	for grew := len(rows) > 0; grew; {
		grew = false
		insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil), (*ast.ValueSpec)(nil)}, func(n ast.Node) {
			var lhs, rhs []ast.Expr
			switch s := n.(type) {
			case *ast.AssignStmt:
				lhs, rhs = s.Lhs, s.Rhs
			case *ast.ValueSpec:
				for _, name := range s.Names {
					lhs = append(lhs, name)
				}
				rhs = s.Values
			}
			if len(lhs) != len(rhs) {
				return
			}
			for i := range rhs {
				id, ok := lhs[i].(*ast.Ident)
				if !ok || !isRowVar(pass, rows, rhs[i]) {
					continue
				}
				if obj := pass.TypesInfo.ObjectOf(id); obj != nil && !rows[obj] {
					rows[obj] = true
					grew = true
				}
			}
		})
	}
	return rows
}

// isEachCall reports whether e calls gotest.Each, type arguments or not.
func isEachCall(pass *analysis.Pass, e ast.Expr) bool {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok {
		return false
	}
	fun := ast.Unparen(call.Fun)
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}
	var id *ast.Ident
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return false
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	return ok && fn.Name() == "Each" && fn.Pkg() != nil && fn.Pkg().Path() == gotestImportPath
}

// isRowVar reports whether e names a variable of rows.
func isRowVar(pass *analysis.Pass, rows map[types.Object]bool, e ast.Expr) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	return ok && rows[pass.TypesInfo.Uses[id]]
}

// onDifferentRows reports whether exactly one of two different T's is an
// Each row's T.
func onDifferentRows(pass *analysis.Pass, rows map[types.Object]bool, a, b ast.Expr) bool {
	if sameExprText(pass, a, b) {
		return false
	}
	return isRowVar(pass, rows, a) != isRowVar(pass, rows, b)
}

// onEachRow reports whether a branch of the guard fails an Each row's T.
func onEachRow(pass *analysis.Pass, rows map[types.Object]bool, units []failGuardUnit) bool {
	for _, u := range units {
		if isRowVar(pass, rows, u.tArg) {
			return true
		}
	}
	return false
}

// keptOnEachRow reports whether the rewrite keeps a guard on an Each row's
// T as it is: one condition whose Fail or Errorf goes on, as the assertion
// does. FailNow or a return ends the method, and several assertions all
// run where the guard stopped at the first. A kept Errorf reads as halting,
// so the finding claims no difference and carries the fix.
func keptOnEachRow(units []failGuardUnit) bool {
	u := &units[0]
	if len(units) > 1 || len(splitOr(u.cond)) > 1 {
		return false
	}
	if u.returns || (u.label != "Fail" && u.label != "Errorf") {
		return false
	}
	u.halting = true
	return true
}
