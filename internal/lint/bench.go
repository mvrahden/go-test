package lint

import (
	"go/ast"
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

// checkBenchFixtureIO flags a suite Benchmark* method on a suite that both
// defines BeforeEach and has at least one fixture-typed field: BeforeEach
// re-running against fixture-backed state (e.g. a database, a file) on
// every benchmark iteration's per-method setup means the timed loop can
// include I/O the benchmark author probably didn't intend to measure.
func checkBenchFixtureIO(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		suite, ok := suites[recvName]
		if !ok || !suite.hasFixtureField {
			return
		}
		if _, hasBeforeEach := suite.methods["BeforeEach"]; !hasBeforeEach {
			return
		}

		methodName := fd.Name.Name
		if !isBenchmarkMethodName(methodName) {
			return
		}

		report(pass, BenchFixtureIO, fd.Pos(),
			"benchmark %s runs BeforeEach against fixture-backed state per method — measurements may include I/O",
			recvName+"."+methodName)
	})
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

// structHasFixtureField reports whether typ (a suite's TypeSpec.Type)
// is a struct with at least one field typed as a pointer to something
// whose name ends in "Fixture" (which also covers the "SharedFixture"
// naming convention, since that suffix ends in "Fixture" too).
func structHasFixtureField(typ ast.Expr) bool {
	st, ok := typ.(*ast.StructType)
	if !ok || st.Fields == nil {
		return false
	}
	for _, field := range st.Fields.List {
		if isFixtureFieldType(field.Type) {
			return true
		}
	}
	return false
}

func isFixtureFieldType(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}

	var name string
	switch t := star.X.(type) {
	case *ast.Ident:
		name = t.Name
	case *ast.SelectorExpr:
		name = t.Sel.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			name = id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			name = id.Name
		}
	}

	return strings.HasSuffix(name, protocol.SuffixFixture)
}
