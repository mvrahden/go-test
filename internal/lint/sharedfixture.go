package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// checkSharedFixtureUndeclared flags suite-method uses of a value whose
// type is a named *...SharedFixture the suite never declared. Window
// scheduling starts only the fixtures scheduled suites require through
// their declared pointer fields (directly or via the fixture DAG), so an
// undeclared read may hit a fixture that was never started or is already
// released — it compiles, and then lies at run time. Locally-constructed
// values (composite literals and constructor results in the same function)
// are exempt: a fixture's own self-test builds and drives the fixture
// without any window.
func checkSharedFixtureUndeclared(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	closures := map[string]map[*types.TypeName]bool{}
	insp.Preorder([]ast.Node{(*ast.TypeSpec)(nil)}, func(n ast.Node) {
		ts := n.(*ast.TypeSpec)
		if _, ok := suites[ts.Name.Name]; !ok {
			return
		}
		obj, ok := pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
		if !ok {
			return
		}
		st, ok := obj.Type().Underlying().(*types.Struct)
		if !ok {
			return
		}
		closures[ts.Name.Name] = declaredFixtureClosure(st, structFixtureFieldNames(ts.Type))
	})

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || fd.Recv == nil || len(fd.Recv.List) == 0 {
			return
		}
		recvName := receiverTypeName(fd.Recv)
		closure, ok := closures[recvName]
		if !ok {
			return
		}
		constructed := locallyConstructed(pass, fd.Body)
		reported := map[*types.TypeName]bool{}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj, ok := pass.TypesInfo.Uses[id].(*types.Var)
			if !ok || constructed[obj] {
				return true
			}
			named := fixtureNamedType(obj.Type())
			if named == nil || !strings.HasSuffix(named.Obj().Name(), protocol.SuffixSharedFixture) {
				return true
			}
			tn := named.Obj()
			if closure[tn] || reported[tn] {
				return true
			}
			reported[tn] = true
			report(pass, SharedFixtureUndeclared, id.Pos(),
				"suite %s uses *%s without declaring it — window scheduling starts only declared fixtures, so it may be absent (never started, or already released); declare a pointer field for it or reach it through a declared fixture",
				recvName, tn.Name())
			return true
		})
	})
}

// locallyConstructed collects the variables body assigns from a composite
// literal or a call result — values built inside this function rather than
// handed over by the scheduler — plus direct aliases of those.
func locallyConstructed(pass *analysis.Pass, body *ast.BlockStmt) map[*types.Var]bool {
	local := map[*types.Var]bool{}
	obj := func(id *ast.Ident) *types.Var {
		if v, ok := pass.TypesInfo.Defs[id].(*types.Var); ok {
			return v
		}
		if v, ok := pass.TypesInfo.Uses[id].(*types.Var); ok {
			return v
		}
		return nil
	}
	mark := func(lhs ast.Expr) {
		id, ok := lhs.(*ast.Ident)
		if !ok {
			return
		}
		if v := obj(id); v != nil {
			local[v] = true
		}
	}
	isConstructed := func(rhs ast.Expr) bool {
		switch e := rhs.(type) {
		case *ast.CompositeLit, *ast.CallExpr:
			return true
		case *ast.UnaryExpr:
			_, lit := e.X.(*ast.CompositeLit)
			return e.Op == token.AND && lit
		case *ast.Ident:
			v := obj(e)
			return v != nil && local[v]
		}
		return false
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if len(node.Rhs) == 1 && len(node.Lhs) > 1 {
				// Multi-value form: one constructor call feeds every LHS.
				if isConstructed(node.Rhs[0]) {
					for _, lhs := range node.Lhs {
						mark(lhs)
					}
				}
				return true
			}
			for i, rhs := range node.Rhs {
				if i < len(node.Lhs) && isConstructed(rhs) {
					mark(node.Lhs[i])
				}
			}
		case *ast.ValueSpec:
			for i, v := range node.Values {
				if i < len(node.Names) && isConstructed(v) {
					mark(node.Names[i])
				}
			}
		}
		return true
	})
	return local
}
