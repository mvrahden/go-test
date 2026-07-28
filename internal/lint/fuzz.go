package lint

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// checkFuzzDeterminism flags fuzz callbacks (and, one level out, the
// same-package functions/methods they call directly) that read
// nondeterministic process state — time.Now, math/rand{,/v2} functions, or
// os.Getenv. A fuzz corpus is meant to be replayable: an input that failed
// yesterday should fail identically today. Reading the clock, the RNG, or
// the environment breaks that guarantee and confuses coverage guidance,
// which assumes a given input always exercises the same code paths.
//
// Detection is import-path based (not identifier-text based), so an
// aliased "rand" import is still caught. The "one level out" following
// mirrors the receiver-follow technique used by
// gotestast.ClassifyLocalFieldsRaw and this package's own buildMethodReach:
// go exactly one hop into same-package callees, not transitively further.
func checkFuzzDeterminism(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	funcDecls := collectFuncDecls(pass, insp)

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || fd.Recv == nil || len(fd.Recv.List) == 0 {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		if _, ok := suites[recvName]; !ok {
			return
		}
		if !isFuzzMethodName(fd.Name.Name) {
			return
		}

		target := recvName + "." + fd.Name.Name
		reportNondeterministicCalls(pass, fd.Body, target)

		visited := map[*ast.FuncDecl]bool{fd: true}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := resolveSamePackageCall(pass, funcDecls, call)
			if callee == nil || callee.Body == nil || visited[callee] {
				return true
			}
			visited[callee] = true
			reportNondeterministicCalls(pass, callee.Body, target)
			return true
		})
	})
}

// reportNondeterministicCalls walks body and reports every call to
// time.Now, any math/rand or math/rand/v2 function, or os.Getenv.
func reportNondeterministicCalls(pass *analysis.Pass, body ast.Node, target string) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		pkgName, ok := pass.TypesInfo.Uses[id].(*types.PkgName)
		if !ok {
			return true
		}

		path := pkgName.Imported().Path()
		nondeterministic := (path == "time" && sel.Sel.Name == "Now") ||
			path == "math/rand" || path == "math/rand/v2" ||
			(path == "os" && sel.Sel.Name == "Getenv")
		if !nondeterministic {
			return true
		}

		report(pass, FuzzDeterminism, call.Pos(),
			"fuzz target %s reads nondeterministic state (%s) — corpus replay and coverage guidance degrade",
			target, id.Name+"."+sel.Sel.Name)
		return true
	})
}

// collectFuncDecls indexes every function and method declaration in the
// package by its declared object, for one-level same-package call
// resolution (see checkFuzzDeterminism).
func collectFuncDecls(pass *analysis.Pass, insp *inspector.Inspector) map[types.Object]*ast.FuncDecl {
	decls := map[types.Object]*ast.FuncDecl{}
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Name == nil {
			return
		}
		if obj := pass.TypesInfo.Defs[fd.Name]; obj != nil {
			decls[obj] = fd
		}
	})
	return decls
}

// resolveSamePackageCall returns the FuncDecl a call resolves to, if it is
// a function or method declared in this package; nil for stdlib/external
// calls (e.g. time.Now itself never resolves here, since its object isn't
// in funcDecls).
func resolveSamePackageCall(pass *analysis.Pass, funcDecls map[types.Object]*ast.FuncDecl, call *ast.CallExpr) *ast.FuncDecl {
	var ident *ast.Ident
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		ident = fn
	case *ast.SelectorExpr:
		ident = fn.Sel
	}
	if ident == nil {
		return nil
	}
	obj := pass.TypesInfo.Uses[ident]
	if obj == nil {
		return nil
	}
	return funcDecls[obj]
}

// fuzzAdapterNames are gotest's generic Fuzz callback adapters (see
// pkg/gotest/f.go): each takes the *gotest.F and a func whose first
// parameter is the *gotest.T handed to every execution.
var fuzzAdapterNames = map[string]bool{"Fuzz": true, "Fuzz2": true, "Fuzz3": true}

// checkFuzzNoOracle flags a gotest.Fuzz/Fuzz2/Fuzz3 callback whose body
// never uses its *gotest.T param to assert anything — neither by passing it
// as the first argument to a gotest.* function, nor by calling a method on
// it directly (t.Errorf, t.FailNow, t.Skipf, ...). Such a callback only
// catches panics; every other input is silently "fine", which defeats the
// point of property-based fuzzing. This is a simple existence check, not a
// reachability analysis: an error-check-then-return body with zero
// assertions still fires, by design — "no crash" is not a property.
func checkFuzzNoOracle(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || fd.Recv == nil || len(fd.Recv.List) == 0 {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		if _, ok := suites[recvName]; !ok {
			return
		}
		if !isFuzzMethodName(fd.Name.Name) {
			return
		}
		target := recvName + "." + fd.Name.Name

		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			tParam, funcLit := fuzzCallback(pass, call)
			if funcLit == nil {
				return true
			}
			if !hasOracleCall(pass, funcLit.Body, tParam) {
				report(pass, FuzzNoOracle, call.Pos(),
					"fuzz target %s only detects panics — assert a property (round-trip, idempotence, no-crash-plus-invariant)",
					target)
			}
			return true
		})
	})
}

// fuzzCallback reports whether call is a gotest.Fuzz/Fuzz2/Fuzz3 invocation
// and, if so, its callback's *gotest.T parameter name and the callback
// itself.
func fuzzCallback(pass *analysis.Pass, call *ast.CallExpr) (string, *ast.FuncLit) {
	sel := fuzzAdapterSelector(call.Fun)
	if sel == nil || !fuzzAdapterNames[sel.Sel.Name] || !isGotestPkgRef(pass, sel.X) {
		return "", nil
	}
	if len(call.Args) == 0 {
		return "", nil
	}
	funcLit, ok := call.Args[len(call.Args)-1].(*ast.FuncLit)
	if !ok || funcLit.Type.Params == nil || len(funcLit.Type.Params.List) == 0 {
		return "", nil
	}
	param := funcLit.Type.Params.List[0]
	if !isGotestTType(pass, param) || len(param.Names) == 0 {
		return "", nil
	}
	return param.Names[0].Name, funcLit
}

// fuzzAdapterSelector unwraps optional explicit type-argument instantiation
// (gotest.Fuzz[string](...)) to get at the underlying selector expression.
func fuzzAdapterSelector(expr ast.Expr) *ast.SelectorExpr {
	switch fn := expr.(type) {
	case *ast.SelectorExpr:
		return fn
	case *ast.IndexExpr:
		return fuzzAdapterSelector(fn.X)
	case *ast.IndexListExpr:
		return fuzzAdapterSelector(fn.X)
	}
	return nil
}

// hasOracleCall reports whether body contains a call passing tParam as the
// first argument to a gotest.* function, or a direct method call on
// tParam (t.Errorf, t.FailNow, t.Skipf, ...).
func hasOracleCall(pass *analysis.Pass, body *ast.BlockStmt, tParam string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == tParam {
			found = true
			return false
		}
		if isGotestPkgRef(pass, sel.X) && len(call.Args) > 0 {
			if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == tParam {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// checkFuzzSeed flags a suite Fuzz* method that never calls f.Add: with no
// seed corpus, coverage-guided exploration starts from nothing (though the
// table-test seed harvester may still backfill one at generate time).
func checkFuzzSeed(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || fd.Recv == nil || len(fd.Recv.List) == 0 {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		if _, ok := suites[recvName]; !ok {
			return
		}
		if !isFuzzMethodName(fd.Name.Name) {
			return
		}

		param := fuzzParamName(fd)
		if param == "" || fuzzBodyHasAdd(fd.Body, param) {
			return
		}

		report(pass, FuzzSeed, fd.Pos(),
			"fuzz target %s declares no seeds — coverage-guided exploration starts blind (table-test harvesting may still seed it)",
			recvName+"."+fd.Name.Name)
	})
}

// isFuzzMethodName mirrors gotestast's IS_FUZZ classification
// (^(?:X_|F_)?Fuzz.+$): an optional X_/F_ marker prefix, then "Fuzz"
// followed by at least one more character.
func isFuzzMethodName(name string) bool {
	stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
	rest, ok := strings.CutPrefix(stripped, protocol.PrefixFuzz)
	return ok && rest != ""
}

// fuzzParamName returns the name of a Fuzz* method's sole parameter (its
// *gotest.F, by convention), or "" if it has none/is unnamed.
func fuzzParamName(fd *ast.FuncDecl) string {
	if fd.Type.Params == nil || len(fd.Type.Params.List) == 0 {
		return ""
	}
	field := fd.Type.Params.List[0]
	if len(field.Names) == 0 {
		return ""
	}
	return field.Names[0].Name
}

// fuzzBodyHasAdd reports whether body references param.Add anywhere — a
// plain selector check, matching the shape benchBodyUsesLoopOrN uses for
// the equivalent bench-loop check.
func fuzzBodyHasAdd(body *ast.BlockStmt, param string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != param {
			return true
		}
		if sel.Sel.Name == "Add" {
			found = true
			return false
		}
		return true
	})
	return found
}

// isGotestPkgRef reports whether expr is an identifier referring to the
// imported gotest package. The fuzz rules key on the literal gotest.Fuzz*
// adapter calls, so a package-name check is the right discriminator here —
// unlike the assertion surface, which is derived from type information.
func isGotestPkgRef(pass *analysis.Pass, expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	obj := pass.TypesInfo.Uses[id]
	if obj == nil {
		return false
	}
	pkgName, ok := obj.(*types.PkgName)
	if !ok {
		return false
	}
	return pkgName.Imported().Path() == gotestImportPath
}
