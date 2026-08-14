package gotestast

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"golang.org/x/tools/go/packages"
)

// SeedLiteral is one harvested table-test (or literal call-site) argument
// tuple, ready to splice into an `f.Add(...)` call in the generated fuzz
// wrapper. Args holds one Go source expression per fuzz-callback parameter,
// in declaration order.
type SeedLiteral struct {
	Args []string // Go source expressions, e.g. `"single digit"`, `int64(-3)` — one per fuzz-callback param, in order
	Pos  token.Pos
}

// harvestTarget describes one callee a fuzz callback invokes, and how the
// callee's own argument positions map back onto the fuzz callback's
// parameter indices (0-based, after the leading *gotest.T).
type harvestTarget struct {
	funcName    string       // generated Fuzz<Suite>_<Method> name this target belongs to
	callee      *types.Func  // the function invoked by the fuzz callback body
	posToCbIdx  map[int]int  // callee arg position -> fuzz-callback param index
	cbParamType []types.Type // fuzz-callback param types, in declared order
}

// HarvestSeeds inspects pkg's _test.go sources — never production files,
// even though pkg.Syntax contains both — for calls to the function(s) each
// fuzz callback (suite.Fuzzers()) invokes, and harvests literal argument
// tuples from two conservative, syntactic patterns elsewhere in the SAME
// package's test code:
//
//  1. Direct literal calls: any call to a harvested callee whose arguments,
//     at the positions that map onto the fuzz callback's parameters, are
//     themselves basic literals (or a unary minus / typed conversion of
//     one) whose type is identical to the corresponding callback param
//     type.
//  2. gotest.Each table rows: a `for _, v := range gotest.Each(t, []T{...})`
//     loop whose body calls a harvested callee, passing the range value (or
//     one of its struct fields) directly as an argument. Each table row
//     whose corresponding value/field is a qualifying literal contributes
//     one seed; non-literal rows are skipped individually.
//
// Struct-typed fuzz callbacks are out of scope — matching is restricted to
// callbacks whose non-*gotest.T parameters are basic types (string, []byte,
// ints, uints, floats, bool), since only those have codecs for f.Add.
//
// Anything that doesn't fit these shapes is skipped silently: this is a
// best-effort corpus seeding aid, not a soundness guarantee.
func HarvestSeeds(pkg *packages.Package, suites TestSuiteSpecSet) (map[string][]SeedLiteral, error) {
	if pkg == nil || len(suites) == 0 {
		return nil, nil
	}

	var targets []harvestTarget
	fuzzBodies := map[*ast.FuncDecl]bool{}
	for _, ts := range suites {
		for _, fz := range ts.Fuzzers() {
			decl, ok := fz.n.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				continue
			}
			fuzzBodies[decl] = true
			funcName := fmt.Sprintf("Fuzz%s_%s", ts.Identifier(), fz.Identifier())
			targets = append(targets, findHarvestTargets(pkg, decl, funcName)...)
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}

	type seedKey struct {
		funcName string
		key      string
	}
	seen := map[seedKey]bool{}
	result := map[string][]SeedLiteral{}

	add := func(funcName string, args []string, pos token.Pos) {
		key := seedKey{funcName: funcName, key: fmt.Sprintf("%q", args)}
		if seen[key] {
			return
		}
		seen[key] = true
		result[funcName] = append(result[funcName], SeedLiteral{Args: args, Pos: pos})
	}

	for _, file := range pkg.Syntax {
		if !isTestFile(pkg, file) {
			continue // never harvest literal call-sites from production sources
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fuzzBodies[fd] {
				continue
			}
			harvestDirectLiteralCalls(pkg, fd, targets, add)
			harvestEachTableRows(pkg, fd, targets, add)
		}
	}

	for funcName, seeds := range result {
		sort.Slice(seeds, func(i, j int) bool { return seeds[i].Pos < seeds[j].Pos })
		result[funcName] = seeds
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// isTestFile reports whether file's filename ends in "_test.go" — harvesting
// scans test sources only, never production code, even though pkg.Syntax
// (for both Ptest and Pxtest packages) contains both.
func isTestFile(pkg *packages.Package, file *ast.File) bool {
	name := pkg.Fset.Position(file.Pos()).Filename
	return strings.HasSuffix(name, "_test.go")
}

// findHarvestTargets locates the gotest.Fuzz/Fuzz2/Fuzz3 call inside a fuzz
// method's body, and for each non-gotest/testing function its callback
// invokes by forwarding ALL of its own data parameters as plain identifier
// arguments (one level of indirection — no deeper dataflow analysis), builds
// a harvestTarget describing the position mapping.
func findHarvestTargets(pkg *packages.Package, fuzzDecl *ast.FuncDecl, funcName string) []harvestTarget {
	var targets []harvestTarget
	ast.Inspect(fuzzDecl.Body, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn := calleeFuncOf(pkg, ce.Fun)
		if fn == nil || !isGotestFuzzAdapter(fn) || len(ce.Args) == 0 {
			return true
		}
		funcLit, ok := ce.Args[len(ce.Args)-1].(*ast.FuncLit)
		if !ok {
			return true
		}
		sig, ok := pkg.TypesInfo.TypeOf(funcLit).(*types.Signature)
		if !ok || sig.Params().Len() < 1 {
			return true
		}

		var cbParamObjs []*types.Var
		var cbParamType []types.Type
		for i := 1; i < sig.Params().Len(); i++ {
			p := sig.Params().At(i)
			cbParamObjs = append(cbParamObjs, p)
			cbParamType = append(cbParamType, p.Type())
		}
		if len(cbParamObjs) == 0 {
			return true
		}

		seenCallee := map[*types.Func]bool{}
		ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
			ce2, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := calleeFuncOf(pkg, ce2.Fun)
			if callee == nil || isExcludedCalleePkg(callee) || seenCallee[callee] {
				return true
			}
			posToCbIdx, ok := coversAllParams(pkg, ce2, cbParamObjs)
			if !ok {
				return true
			}
			seenCallee[callee] = true
			targets = append(targets, harvestTarget{
				funcName:    funcName,
				callee:      callee,
				posToCbIdx:  posToCbIdx,
				cbParamType: cbParamType,
			})
			return true
		})
		return true
	})
	return targets
}

// coversAllParams reports whether ce's arguments forward every param in
// cbParamObjs exactly once as a bare identifier, returning the callee
// argument-position -> callback-param-index mapping if so.
func coversAllParams(pkg *packages.Package, ce *ast.CallExpr, cbParamObjs []*types.Var) (map[int]int, bool) {
	if ce.Ellipsis != token.NoPos {
		return nil, false // variadic call-site forwarding — out of scope
	}
	posToCbIdx := map[int]int{}
	usedCbIdx := map[int]bool{}
	for pos, arg := range ce.Args {
		ident, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		obj := pkg.TypesInfo.Uses[ident]
		for cbIdx, p := range cbParamObjs {
			if obj == p {
				posToCbIdx[pos] = cbIdx
				usedCbIdx[cbIdx] = true
				break
			}
		}
	}
	if len(usedCbIdx) != len(cbParamObjs) {
		return nil, false
	}
	return posToCbIdx, true
}

// isGotestFuzzAdapter reports whether fn is one of gotest.Fuzz/Fuzz2/Fuzz3.
func isGotestFuzzAdapter(fn *types.Func) bool {
	if fn.Pkg() == nil || fn.Pkg().Path() != about.Repo+"/pkg/gotest" {
		return false
	}
	switch fn.Name() {
	case "Fuzz", "Fuzz2", "Fuzz3":
		return true
	default:
		return false
	}
}

// isExcludedCalleePkg excludes assertion/lifecycle helpers (gotest, testing)
// from candidacy as a harvestable callee — these are never "the function
// under test", so matching against them would harvest assertion-call
// literals as bogus seeds.
func isExcludedCalleePkg(fn *types.Func) bool {
	if fn.Pkg() == nil {
		return false
	}
	path := fn.Pkg().Path()
	return path == about.Repo+"/pkg/gotest" || path == "testing"
}

// calleeFuncOf resolves the *types.Func a call expression's Fun operand
// refers to, unwrapping explicit generic instantiations.
func calleeFuncOf(pkg *packages.Package, fun ast.Expr) *types.Func {
	switch e := fun.(type) {
	case *ast.Ident:
		fn, _ := pkg.TypesInfo.Uses[e].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pkg.TypesInfo.Uses[e.Sel].(*types.Func)
		return fn
	case *ast.IndexExpr:
		return calleeFuncOf(pkg, e.X)
	case *ast.IndexListExpr:
		return calleeFuncOf(pkg, e.X)
	default:
		return nil
	}
}

// harvestDirectLiteralCalls scans fd for calls to a harvested callee whose
// mapped argument positions are all qualifying literals.
func harvestDirectLiteralCalls(pkg *packages.Package, fd *ast.FuncDecl, targets []harvestTarget, add func(funcName string, args []string, pos token.Pos)) {
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := calleeFuncOf(pkg, ce.Fun)
		if callee == nil {
			return true
		}
		for _, t := range targets {
			if t.callee != callee {
				continue
			}
			args, ok := extractLiteralArgs(pkg, ce, t)
			if !ok {
				continue
			}
			add(t.funcName, args, ce.Pos())
		}
		return true
	})
}

func extractLiteralArgs(pkg *packages.Package, ce *ast.CallExpr, t harvestTarget) ([]string, bool) {
	args := make([]string, len(t.cbParamType))
	for pos, cbIdx := range t.posToCbIdx {
		if pos >= len(ce.Args) {
			return nil, false
		}
		src, ok := literalSource(pkg, ce.Args[pos], t.cbParamType[cbIdx])
		if !ok {
			return nil, false
		}
		args[cbIdx] = src
	}
	return args, true
}

// harvestEachTableRows scans fd for `for _, v := range gotest.Each(t,
// []T{...})` loops whose body calls a harvested callee by forwarding the
// range value (or a field of it) directly, and harvests one seed per
// literal-valued table row.
func harvestEachTableRows(pkg *packages.Package, fd *ast.FuncDecl, targets []harvestTarget, add func(funcName string, args []string, pos token.Pos)) {
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		rs, ok := n.(*ast.RangeStmt)
		if !ok || rs.Value == nil {
			return true
		}
		ce, ok := rs.X.(*ast.CallExpr)
		if !ok || len(ce.Args) < 2 {
			return true
		}
		eachFn := calleeFuncOf(pkg, ce.Fun)
		if eachFn == nil || eachFn.Pkg() == nil || eachFn.Pkg().Path() != about.Repo+"/pkg/gotest" || eachFn.Name() != "Each" {
			return true
		}
		valueIdent, ok := rs.Value.(*ast.Ident)
		if !ok {
			return true
		}
		valueObj := pkg.TypesInfo.Defs[valueIdent]
		if valueObj == nil {
			return true
		}
		entriesLit, ok := ce.Args[1].(*ast.CompositeLit)
		if !ok {
			return true
		}
		elemType := pkg.TypesInfo.TypeOf(entriesLit)
		if sl, ok := elemType.(*types.Slice); ok {
			elemType = sl.Elem()
		} else if at, ok := elemType.(*types.Array); ok {
			elemType = at.Elem()
		} else {
			return true
		}

		if rs.Body == nil {
			return true
		}
		ast.Inspect(rs.Body, func(inner ast.Node) bool {
			ce2, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := calleeFuncOf(pkg, ce2.Fun)
			if callee == nil {
				return true
			}
			for _, t := range targets {
				if t.callee != callee {
					continue
				}
				extraction, ok := matchRowExtraction(pkg, ce2, t, valueObj)
				if !ok {
					continue
				}
				for _, entry := range entriesLit.Elts {
					args, ok := extractRow(pkg, elemType, entry, t, extraction)
					if !ok {
						continue // non-literal row — skipped individually
					}
					add(t.funcName, args, entry.Pos())
				}
			}
			return true
		})
		return true
	})
}

// rowExtract describes how to pull one fuzz-callback argument's value out of
// a gotest.Each table row: either the row's whole value, or one named field
// of it.
type rowExtract struct {
	whole bool
	field string
}

// matchRowExtraction reports whether ce2's arguments, at t's mapped
// positions, are the Each range value itself or a selector on it — and if
// so, returns the callback-param-index -> rowExtract mapping.
func matchRowExtraction(pkg *packages.Package, ce2 *ast.CallExpr, t harvestTarget, valueObj types.Object) (map[int]rowExtract, bool) {
	out := map[int]rowExtract{}
	for pos, cbIdx := range t.posToCbIdx {
		if pos >= len(ce2.Args) {
			return nil, false
		}
		switch a := ce2.Args[pos].(type) {
		case *ast.Ident:
			if pkg.TypesInfo.Uses[a] != valueObj {
				return nil, false
			}
			out[cbIdx] = rowExtract{whole: true}
		case *ast.SelectorExpr:
			ident, ok := a.X.(*ast.Ident)
			if !ok || pkg.TypesInfo.Uses[ident] != valueObj {
				return nil, false
			}
			out[cbIdx] = rowExtract{field: a.Sel.Name}
		default:
			return nil, false
		}
	}
	return out, true
}

func extractRow(pkg *packages.Package, elemType types.Type, entry ast.Expr, t harvestTarget, extraction map[int]rowExtract) ([]string, bool) {
	args := make([]string, len(t.cbParamType))
	for cbIdx, ext := range extraction {
		valExpr, ok := entryFieldExpr(elemType, entry, ext)
		if !ok {
			return nil, false
		}
		src, ok := literalSource(pkg, valExpr, t.cbParamType[cbIdx])
		if !ok {
			return nil, false
		}
		args[cbIdx] = src
	}
	return args, true
}

func entryFieldExpr(elemType types.Type, entry ast.Expr, ext rowExtract) (ast.Expr, bool) {
	if ext.whole {
		return entry, true
	}
	cl, ok := entry.(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	st, ok := elemType.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	// keyed struct literal: {Field: value, ...}
	anyKeyed := false
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			anyKeyed = true
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == ext.field {
				return kv.Value, true
			}
		}
	}
	if anyKeyed {
		return nil, false // keyed literal without the field we need
	}
	// unkeyed struct literal: positional
	idx := -1
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Name() == ext.field {
			idx = i
			break
		}
	}
	if idx == -1 || idx >= len(cl.Elts) {
		return nil, false
	}
	return cl.Elts[idx], true
}

// literalSource reports whether expr is a qualifying constant literal — a
// basic literal, a unary minus of one, a predeclared bool literal, or a
// single-argument typed conversion of one of those — whose static type is
// types.Identical to want. On success it returns expr's exact Go source
// text.
func literalSource(pkg *packages.Package, expr ast.Expr, want types.Type) (string, bool) {
	if !isLiteralShape(pkg, expr) {
		return "", false
	}
	got := pkg.TypesInfo.TypeOf(expr)
	if got == nil || want == nil || !types.Identical(got, want) {
		return "", false
	}
	return renderExpr(pkg.Fset, expr)
}

func isLiteralShape(pkg *packages.Package, expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.UnaryExpr:
		if e.Op != token.SUB {
			return false
		}
		_, ok := e.X.(*ast.BasicLit)
		return ok
	case *ast.Ident:
		return e.Name == "true" || e.Name == "false"
	case *ast.CallExpr:
		if len(e.Args) != 1 || e.Ellipsis != token.NoPos {
			return false
		}
		tv, ok := pkg.TypesInfo.Types[e.Fun]
		if !ok || !tv.IsType() {
			return false // not a type conversion — an ordinary func call is not a literal
		}
		switch e.Args[0].(type) {
		case *ast.BasicLit, *ast.UnaryExpr, *ast.Ident:
			return isLiteralShape(pkg, e.Args[0])
		default:
			return false
		}
	default:
		return false
	}
}

func renderExpr(fset *token.FileSet, expr ast.Expr) (string, bool) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return "", false
	}
	return buf.String(), true
}
