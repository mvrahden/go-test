package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/typeclass"
)

// caseHandles maps a case prefix to the handle its case takes.
var caseHandles = map[string]typeclass.Handle{
	"Test":                   typeclass.TestHandle,
	protocol.PrefixBenchmark: typeclass.BenchHandle,
	protocol.PrefixFuzz:      typeclass.FuzzHandle,
}

// checkDroppedCase reports Test, Benchmark and Fuzz methods no suite runs: on
// a type of a test file that is no suite, named by a case prefix alone on a
// suite, or on a generic suite of a test file that no alias instantiates. A
// method either test package of the directory refers to, directly or
// through an interface, is a helper; a type an alias of either names as a
// suite runs.
//
// gotest lint loads both test packages of a directory and judges each case
// as the generator does, over both (casesOfDir). A driver that hands the
// analyzer one test package at a time (go vet, golangci-lint) cannot see
// the other: there the rule reports only in a directory whose test files are
// all in the package it sees, and leaves the rest to the generator, which
// refuses such a package when it runs it.
func checkDroppedCase(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	v := casesOfDir(pass.Pkg)
	if v == nil {
		if !onlyTestPackage(pass) {
			return
		}
		v = newCaseUses(variant{pass.TypesInfo, pass.Files})
	}
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil), (*ast.TypeSpec)(nil)}, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.FuncDecl:
			checkDroppedMethod(pass, n, suites, v)
		case *ast.TypeSpec:
			checkUnaliasedGeneric(pass, n, suites, v.aliased)
		}
	})
}

func checkDroppedMethod(pass *analysis.Pass, fd *ast.FuncDecl, suites map[string]*suiteInfo, v *caseUses) {
	if fd.Recv == nil || !fd.Name.IsExported() {
		return
	}
	fn, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func)
	if !ok || v.called[fn] || v.byName[fd.Name.Name] {
		return
	}
	named := namedReceiver(fn)
	if named == nil {
		return
	}
	name := fd.Name.Name
	stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
	prefix := casePrefixOf(stripped)
	if prefix == "" || fn.Signature().Params().Len() == 0 {
		return
	}
	h, stdlib := typeclass.CaseHandle(fn.Signature().Params().At(0).Type())
	if h != caseHandles[prefix] {
		return
	}
	typ := named.Obj().Name()
	bare := stripped == prefix
	isSuite := suites[typ] != nil || v.aliased[named.Obj()]
	switch {
	case isSuite && bare:
		report(pass, DroppedCase, funcHeader(fd),
			"%s on %s never runs: a case needs a name after %s; rename it, or give the helper a name that does not begin with %s",
			name, typ, prefix, prefix)
	case isSuite || stdlib || gotestast.IS_TEST_SUITE.MatchString(typ) || isTestifySuite(pass, typ):
	case !bare && !isCaseMethodName(name):
	case !inTestFile(pass, fd.Pos()):
	case v.orphans != nil && !v.orphans[named.Obj()]:
		// The generator's own verdict over both test packages runs it.
	default:
		report(pass, DroppedCase, funcHeader(fd),
			"%s on %s never runs: %s is no suite; end its name in TestSuite, or give the helper a name that does not begin with %s",
			name, typ, typ, prefix)
	}
}

// checkUnaliasedGeneric reports a generic suite of a test file with a case
// and no alias naming a suite that instantiates it.
func checkUnaliasedGeneric(pass *analysis.Pass, ts *ast.TypeSpec, suites map[string]*suiteInfo, aliased map[*types.TypeName]bool) {
	if ts.TypeParams == nil || ts.Assign.IsValid() || suites[ts.Name.Name] == nil || !inTestFile(pass, ts.Pos()) {
		return
	}
	obj, ok := pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
	if !ok || aliased[obj] {
		return
	}
	named, ok := obj.Type().(*types.Named)
	if !ok || !hasCaseMethod(named) {
		return
	}
	report(pass, DroppedCase, span{ts.Name.Pos(), ts.Name.End()},
		"generic suite %s never runs: no alias instantiates it; declare one (type …TestSuite = %s[…]), or delete it",
		ts.Name.Name, ts.Name.Name)
}

// namedReceiver is the named type behind fn's receiver.
func namedReceiver(fn *types.Func) *types.Named {
	t := fn.Signature().Recv().Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, _ := types.Unalias(t).(*types.Named)
	if named == nil {
		return nil
	}
	return named.Origin()
}

// casePrefixOf is the case prefix name begins with, "" for none.
func casePrefixOf(name string) string {
	for _, p := range []string{"Test", protocol.PrefixBenchmark, protocol.PrefixFuzz} {
		if strings.HasPrefix(name, p) {
			return p
		}
	}
	return ""
}

// isCaseMethodName reports whether name, an X_ or F_ marker aside, is a case
// name with something after its prefix, as the generator reads one.
func isCaseMethodName(name string) bool {
	return gotestast.IS_TEST_CASE.MatchString(name) || gotestast.IS_BENCHMARK.MatchString(name) || gotestast.IS_FUZZ.MatchString(name)
}

func hasCaseMethod(named *types.Named) bool {
	for i := range named.NumMethods() {
		if isCaseMethodName(named.Method(i).Name()) {
			return true
		}
	}
	return false
}

// inTestFile reports whether pos lies in a _test.go file gotest did not
// generate.
func inTestFile(pass *analysis.Pass, pos token.Pos) bool {
	name := pass.Fset.File(pos).Name()
	return strings.HasSuffix(name, "_test.go") && !about.PSuiteRegex.MatchString(name)
}
