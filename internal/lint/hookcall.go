package lint

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/protocol"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// harnessHooks are the lifecycle hooks the generated harness calls.
var harnessHooks = map[string]bool{
	"BeforeAll": true, "AfterAll": true, "BeforeEach": true, "AfterEach": true,
	"Hydrate": true, "Dehydrate": true,
}

// checkFixtureHookCall reports a suite or fixture method calling a hook of
// its own or of a fixture it holds: the harness calls it too. A fixture the
// method builds itself is not the harness's, so its calls are not reported.
func checkFixtureHookCall(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Recv == nil || len(fd.Recv.List) != 1 || len(fd.Recv.List[0].Names) != 1 || fd.Body == nil {
			return
		}
		if !harnessType(receiverTypeName(fd.Recv)) {
			return
		}
		recv := pass.TypesInfo.Defs[fd.Recv.List[0].Names[0]]
		if recv == nil {
			return
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
			if !ok || !harnessHooks[sel.Sel.Name] {
				return true
			}
			if s := pass.TypesInfo.Selections[sel]; s == nil || s.Kind() != types.MethodVal || !harnessType(recvNamed(s.Obj())) {
				return true
			}
			if rootIdent(pass, sel.X) == recv {
				report(pass, FixtureHookCall, call.Pos(), "%s is called by the harness; calling it here runs it twice", sel.Sel.Name)
			}
			return true
		})
	})
}

// harnessType reports whether name is a suite or fixture type name.
func harnessType(name string) bool {
	return name != "" && (gotestast.IS_TEST_SUITE.MatchString(name) || strings.HasSuffix(name, protocol.SuffixFixture))
}

// recvNamed names the type a method is declared on, or "".
func recvNamed(obj types.Object) string {
	fn, ok := obj.(*types.Func)
	if !ok {
		return ""
	}
	recv := fn.Signature().Recv()
	if recv == nil {
		return ""
	}
	t := types.Unalias(recv.Type())
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

// rootIdent is the object a selector chain such as s.Store.DB starts at.
func rootIdent(pass *analysis.Pass, e ast.Expr) types.Object {
	for {
		switch x := ast.Unparen(e).(type) {
		case *ast.SelectorExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.Ident:
			return pass.TypesInfo.Uses[x]
		default:
			return nil
		}
	}
}
