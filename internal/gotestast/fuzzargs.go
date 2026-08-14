package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// FuzzArg is one instantiated type argument of a gotest.Fuzz/Fuzz2/Fuzz3
// call inside a suite fuzz method.
//
// It is read from types.Info.Instances — the instantiation, never the
// callback body — so the callback does not have to be an inline function
// literal: a method value or a named function works just as well.
type FuzzArg struct {
	FuncName string     // generated wrapper name, e.g. "FuzzUserTestSuite_FuzzCreate"
	Adapter  string     // "Fuzz", "Fuzz2" or "Fuzz3"
	Index    int        // 0-based position among the fuzzed arguments
	Type     types.Type // the instantiated type argument
	Pos      token.Pos  // position of the adapter call
}

// CollectFuzzArgs walks every suite fuzz method's body for gotest.Fuzz
// adapter calls and returns their type arguments in deterministic
// (suite, method, call, argument) order. Returns nil when the package fuzzes
// nothing.
func CollectFuzzArgs(pkg *packages.Package, suites TestSuiteSpecSet) []FuzzArg {
	if pkg == nil || pkg.TypesInfo == nil || len(suites) == 0 {
		return nil
	}

	var out []FuzzArg
	for _, ts := range suites {
		for _, fz := range ts.Fuzzers() {
			decl, ok := fz.n.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				continue
			}
			funcName := fmt.Sprintf("Fuzz%s_%s", ts.Identifier(), fz.Identifier())
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				fn := calleeFuncOf(pkg, ce.Fun)
				if fn == nil || !isGotestFuzzAdapter(fn) {
					return true
				}
				ident := calleeIdentOf(ce.Fun)
				if ident == nil {
					return true
				}
				inst, ok := pkg.TypesInfo.Instances[ident]
				if !ok || inst.TypeArgs == nil {
					return true
				}
				for i := 0; i < inst.TypeArgs.Len(); i++ {
					out = append(out, FuzzArg{
						FuncName: funcName,
						Adapter:  fn.Name(),
						Index:    i,
						Type:     inst.TypeArgs.At(i),
						Pos:      ce.Pos(),
					})
				}
				return true
			})
		}
	}
	return out
}
