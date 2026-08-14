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

// checkBenchLoop flags a suite Benchmark* method whose body never touches
// b.Loop() or b.N — i.e. it never actually iterates, so nothing is
// measured. Only pointer-receiver methods on a discovered suite are
// considered; a non-pointer-receiver suite method is already flagged by
// the Receiver rule and would never be dispatched as a benchmark anyway.
func checkBenchLoop(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || !isPointerReceiver(fd.Recv) {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		if _, ok := suites[recvName]; !ok {
			return
		}

		methodName := fd.Name.Name
		if !isBenchmarkMethodName(methodName) {
			return
		}

		param := benchParamName(fd)
		if param == "" || benchBodyUsesLoopOrN(fd.Body, param) {
			return
		}

		report(pass, BenchLoop, fd.Pos(),
			"benchmark %s never calls b.Loop() — nothing is measured", recvName+"."+methodName)
	})
}

// checkBenchFixtureIO flags a suite Benchmark* method that reads a
// fixture-typed field from inside its measured loop. gotest's generated
// wrapper fences the timer around BeforeEach/AfterEach (b.StopTimer() ...
// BeforeEach ... b.StartTimer(); b.ResetTimer(); method), so fixture setup
// itself is never measured — that structural guarantee is what the docs
// promise. What it cannot save you from is the benchmark method's own body
// reading fixture-backed state *inside* the loop: if the fixture is backed
// by a database or a network service, that read times whatever backs the
// fixture, not the code under test.
func checkBenchFixtureIO(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || !isPointerReceiver(fd.Recv) {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		suite, ok := suites[recvName]
		if !ok || len(suite.fixtureFields) == 0 {
			return
		}

		methodName := fd.Name.Name
		if !isBenchmarkMethodName(methodName) {
			return
		}

		recvIdent := receiverIdentName(fd.Recv)
		if recvIdent == "" || recvIdent == "_" {
			return
		}

		param := benchParamName(fd)
		if param == "" || param == "_" {
			return
		}

		region := benchMeasuredRegion(fd.Body, param)
		if region == nil {
			// No b.Loop()/b.N loop found — the bench-loop rule already
			// covers that case; nothing to report here.
			return
		}

		field := findFixtureRead(region, recvIdent, suite.fixtureFields)
		if field == "" {
			return
		}

		report(pass, BenchFixtureIO, fd.Pos(),
			"benchmark %s reads fixture-backed state %s inside the measured loop — hoist the read above the loop, or you are timing whatever backs the fixture",
			recvName+"."+methodName, recvIdent+"."+field)
	})
}

// checkBenchWait flags waiting primitives inside the measured loop of a
// Benchmark* suite method: time.Sleep and gotest's Eventually/Consistently
// pollers. The loop then times the wait, not the code — a result that says
// nothing about the operation being benchmarked. Settling belongs above
// the loop; a property that needs polling belongs in a test, not a
// benchmark.
func checkBenchWait(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || !isPointerReceiver(fd.Recv) {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		if _, ok := suites[recvName]; !ok {
			return
		}

		methodName := fd.Name.Name
		if !isBenchmarkMethodName(methodName) {
			return
		}

		param := benchParamName(fd)
		if param == "" {
			return
		}
		region := benchMeasuredRegion(fd.Body, param)
		if region == nil {
			return
		}

		ast.Inspect(region, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name := waitingCallName(pass, call); name != "" {
				report(pass, BenchWait, call.Pos(),
					"benchmark %s calls %s inside the measured loop — this times the wait, not the code; move it outside the loop",
					recvName+"."+methodName, name)
			}
			return true
		})
	})
}

// waitingCallName resolves call to a known waiting primitive — time.Sleep
// (matched by import path, so aliased imports stay covered) or gotest's
// Eventually/Consistently — and returns its display name, or "".
func waitingCallName(pass *analysis.Pass, call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if id, ok := sel.X.(*ast.Ident); ok && sel.Sel.Name == "Sleep" {
		if pkgName, ok := pass.TypesInfo.Uses[id].(*types.PkgName); ok && pkgName.Imported().Path() == "time" {
			return "time.Sleep"
		}
	}
	if fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func); ok && fn.Pkg() != nil && fn.Pkg().Path() == gotestImportPath {
		if fn.Name() == "Eventually" || fn.Name() == "Consistently" {
			return "gotest." + fn.Name()
		}
	}
	return ""
}

// receiverIdentName returns the receiver's identifier name (e.g. "s" in
// "func (s *Suite) Method()"), or "" if the receiver has no name.
func receiverIdentName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 || len(recv.List[0].Names) == 0 {
		return ""
	}
	return recv.List[0].Names[0].Name
}

// benchMeasuredRegion returns the body of the benchmark method's timed
// loop: either `for <param>.Loop() { ... }` or the classic
// `for i := 0; i < <param>.N; i++ { ... }` form. It searches the whole
// method body (not just its top level), and returns nil if no such loop is
// found — the bench-loop rule already flags that shape.
//
// Only the first such loop is returned. A benchmark with two of them is not
// merely unusual: a second b.Loop() panics at run time with "B.Loop called
// with timer stopped", so the shape cannot survive execution long enough for
// a missed diagnostic to matter.
func benchMeasuredRegion(body *ast.BlockStmt, param string) *ast.BlockStmt {
	var region *ast.BlockStmt
	ast.Inspect(body, func(n ast.Node) bool {
		if region != nil {
			return false
		}
		forStmt, ok := n.(*ast.ForStmt)
		if !ok {
			return true
		}
		if isBenchLoopCond(forStmt.Cond, param) || isBenchNCond(forStmt.Cond, param) {
			region = forStmt.Body
			return false
		}
		return true
	})
	return region
}

// isBenchLoopCond reports whether cond is a call to <param>.Loop().
func isBenchLoopCond(cond ast.Expr, param string) bool {
	call, ok := cond.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == param && sel.Sel.Name == "Loop"
}

// isBenchNCond reports whether cond is the classic `i < <param>.N` form.
func isBenchNCond(cond ast.Expr, param string) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.LSS {
		return false
	}
	sel, ok := bin.Y.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == param && sel.Sel.Name == "N"
}

// findFixtureRead walks region — including nested loops and closures —
// for the first selector expression of the form <recv>.<field> where field
// is a known fixture-typed field, returning that field's name ("" if none
// is found).
func findFixtureRead(region *ast.BlockStmt, recv string, fields map[string]bool) string {
	found := ""
	ast.Inspect(region, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != recv {
			return true
		}
		if fields[sel.Sel.Name] {
			found = sel.Sel.Name
			return false
		}
		return true
	})
	return found
}

// isBenchmarkMethodName mirrors gotestast's IS_BENCHMARK classification
// (^(?:X_|F_)?Benchmark.+$): an optional X_/F_ marker prefix, then
// "Benchmark" followed by at least one more character.
func isBenchmarkMethodName(name string) bool {
	stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
	rest, ok := strings.CutPrefix(stripped, protocol.PrefixBenchmark)
	return ok && rest != ""
}

// benchParamName returns the name of a benchmark method's sole parameter
// (its *gotest.B, by convention), or "" if it has none/is unnamed.
func benchParamName(fd *ast.FuncDecl) string {
	if fd.Type.Params == nil || len(fd.Type.Params.List) == 0 {
		return ""
	}
	field := fd.Type.Params.List[0]
	if len(field.Names) == 0 {
		return ""
	}
	return field.Names[0].Name
}

// benchBodyUsesLoopOrN reports whether body references param.Loop or
// param.N anywhere — a plain selector check, not requiring Loop to
// actually be called, matching the shape used elsewhere in this package
// for similar heuristics (e.g. isTMethodCall).
func benchBodyUsesLoopOrN(body *ast.BlockStmt, param string) bool {
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
		if sel.Sel.Name == "Loop" || sel.Sel.Name == "N" {
			found = true
			return false
		}
		return true
	})
	return found
}

// Fixture-typed field discovery lives in fixturefields.go, shared with the
// shared-fixture-undeclared rule: discoverSuites feeds suiteInfo.fixtureFields
// through structFixtureFieldNames there.
