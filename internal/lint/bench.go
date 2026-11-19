package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
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
	reach := buildLoopReach(pass, insp, 5)
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
		if param == "" || benchBodyUsesLoopOrN(fd.Body, param) || reach.handsOff(fd.Body, newWrapperRecv(fd.Body, param, "B")) {
			return
		}

		report(pass, BenchLoop, funcHeader(fd),
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

		report(pass, BenchFixtureIO, funcHeader(fd),
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
				report(pass, BenchWait, call,
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
	recv := newWrapperRecv(body, param, "B")
	var region *ast.BlockStmt
	ast.Inspect(body, func(n ast.Node) bool {
		if region != nil {
			return false
		}
		forStmt, ok := n.(*ast.ForStmt)
		if !ok {
			return true
		}
		if isBenchLoopCond(forStmt.Cond, recv) || isBenchNCond(forStmt.Cond, recv) {
			region = forStmt.Body
			return false
		}
		return true
	})
	return region
}

// isBenchLoopCond reports whether cond is a call to Loop() on the parameter,
// its escape or an alias of it: b.Loop(), b.B().Loop(), tb.Loop().
func isBenchLoopCond(cond ast.Expr, recv wrapperRecv) bool {
	call, ok := cond.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return recv.matches(sel.X) && sel.Sel.Name == "Loop"
}

// isBenchNCond reports whether cond is the classic `i < <param>.N` form, on
// the parameter, its escape or an alias of it.
func isBenchNCond(cond ast.Expr, recv wrapperRecv) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.LSS {
		return false
	}
	sel, ok := bin.Y.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return recv.matches(sel.X) && sel.Sel.Name == "N"
}

// wrapperRecv matches what stands for a wrapper parameter in a method body:
// the parameter, its escape <param>.<wrapper>() — b.B() or f.F() — or a name
// assigned from that escape. Syntactic: a shadowed name is not told apart.
type wrapperRecv struct {
	param, wrapper string
	aliases        map[string]bool
}

// newWrapperRecv scans body once for names assigned from the escape:
// x := b.B(), x = b.B(), var x = b.B().
func newWrapperRecv(body *ast.BlockStmt, param, wrapper string) wrapperRecv {
	recv := wrapperRecv{param: param, wrapper: wrapper, aliases: map[string]bool{}}
	alias := func(names, values []ast.Expr) {
		if len(names) != len(values) {
			return
		}
		for i, v := range values {
			if id, ok := names[i].(*ast.Ident); ok && id.Name != "_" && recv.isEscape(v) {
				recv.aliases[id.Name] = true
			}
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			alias(n.Lhs, n.Rhs)
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(n.Names))
			for i, id := range n.Names {
				names[i] = id
			}
			alias(names, n.Values)
		}
		return true
	})
	return recv
}

// matches reports whether expr is the parameter, its escape or an alias.
func (w wrapperRecv) matches(expr ast.Expr) bool {
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name == w.param || w.aliases[id.Name]
	}
	return w.isEscape(expr)
}

// isEscape reports whether expr is the zero-argument call <param>.<wrapper>().
func (w wrapperRecv) isEscape(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != w.wrapper {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == w.param
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

// isBenchmarkMethodName mirrors gotestast's IS_BENCHMARK classification: an
// optional X_/F_ marker prefix, then "Benchmark" followed by at least one
// more character that go test's rule accepts (not a lowercase letter).
func isBenchmarkMethodName(name string) bool {
	stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
	rest, ok := strings.CutPrefix(stripped, protocol.PrefixBenchmark)
	return ok && rest != "" && gotestast.IsGoTestName(stripped)
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

// benchBodyUsesLoopOrN reports whether body references Loop or N on the
// parameter, its escape or an alias anywhere — a plain selector check, not
// requiring Loop to actually be called, matching the shape used elsewhere
// in this package for similar heuristics (e.g. wrapperEscape).
func benchBodyUsesLoopOrN(body *ast.BlockStmt, param string) bool {
	recv := newWrapperRecv(body, param, "B")
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if !recv.matches(sel.X) {
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

// loopReach marks the package's functions that loop on a parameter: they
// name Loop or N on it, or hand it to one that does.
type loopReach struct {
	pass  *analysis.Pass
	loops map[*types.Func]map[int]bool // parameter index → loops
	known map[*types.Func]bool         // declared with a body in this package
}

// buildLoopReach reads every function of the package once, then follows
// hand-offs between them for at most maxDepth rounds.
func buildLoopReach(pass *analysis.Pass, insp *inspector.Inspector, maxDepth int) *loopReach {
	r := &loopReach{pass: pass, loops: map[*types.Func]map[int]bool{}, known: map[*types.Func]bool{}}
	type benchParam struct {
		fn   *types.Func
		body *ast.BlockStmt
		name string
		idx  int
	}
	var pending []benchParam
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		fn, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func)
		if !ok || fd.Body == nil {
			return
		}
		r.known[fn] = true
		for name, idx := range flattenParams(fd.Type.Params) {
			if name == "_" || !isBenchHandle(pass.TypesInfo.TypeOf(paramIdent(fd, name))) {
				continue
			}
			if benchBodyUsesLoopOrN(fd.Body, name) {
				r.mark(fn, idx)
				continue
			}
			pending = append(pending, benchParam{fn, fd.Body, name, idx})
		}
	})
	for range maxDepth {
		marked := false
		rest := pending[:0]
		for _, p := range pending {
			if r.handOff(p.body, newWrapperRecv(p.body, p.name, "B"), false) {
				r.mark(p.fn, p.idx)
				marked = true
				continue
			}
			rest = append(rest, p)
		}
		pending = rest
		if !marked {
			break
		}
	}
	return r
}

func (r *loopReach) mark(fn *types.Func, idx int) {
	if r.loops[fn] == nil {
		r.loops[fn] = map[int]bool{}
	}
	r.loops[fn][idx] = true
}

// handsOff reports whether body passes what recv matches to a call that
// loops on it, or to one lint cannot read.
func (r *loopReach) handsOff(body *ast.BlockStmt, recv wrapperRecv) bool {
	return r.handOff(body, recv, true)
}

// handOff is handsOff; unread counts a callee lint cannot read as a
// hand-off. gotest's own functions are never one: they assert, they do not
// loop.
func (r *loopReach) handOff(body *ast.BlockStmt, recv wrapperRecv, unread bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for i, arg := range call.Args {
			if !recv.matches(arg) {
				continue
			}
			if r.callLoops(call, i, unread) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// callLoops reports whether call's argument i reaches a loop.
func (r *loopReach) callLoops(call *ast.CallExpr, i int, unread bool) bool {
	var id *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = fun
	case *ast.SelectorExpr:
		id = fun.Sel
	case *ast.IndexExpr: // an instantiated generic function
		id = calleeIdent(fun.X)
	case *ast.IndexListExpr:
		id = calleeIdent(fun.X)
	}
	var obj types.Object
	if id != nil {
		obj = r.pass.TypesInfo.Uses[id]
	}
	switch obj := obj.(type) {
	case *types.Func:
		fn := obj.Origin()
		switch {
		case fn.Pkg() == r.pass.Pkg:
			if !r.known[fn] {
				return unread // an interface method
			}
			if sig, ok := fn.Type().(*types.Signature); ok && sig.Variadic() && i >= sig.Params().Len()-1 {
				i = sig.Params().Len() - 1
			}
			return r.loops[fn][i]
		case fn.Pkg() != nil && fn.Pkg().Path() == gotestImportPath:
			return false
		}
		return unread
	case *types.Builtin, *types.TypeName:
		return false // append, a conversion
	}
	return unread // a function value
}

func calleeIdent(e ast.Expr) *ast.Ident {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return e.Sel
	}
	return nil
}

// paramIdent is the identifier that declares parameter name of fd.
func paramIdent(fd *ast.FuncDecl, name string) *ast.Ident {
	for _, field := range fd.Type.Params.List {
		for _, id := range field.Names {
			if id.Name == name {
				return id
			}
		}
	}
	return nil
}

// isBenchHandle reports whether t is *gotest.B or *testing.B.
func isBenchHandle(t types.Type) bool {
	return namedPtrType(t, gotestImportPath, "B") || namedPtrType(t, "testing", "B")
}

// Fixture-typed field discovery lives in fixturefields.go, shared with the
// shared-fixture-undeclared rule: discoverSuites feeds suiteInfo.fixtureFields
// through structFixtureFieldNames there.
