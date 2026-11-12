package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
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
// method the package calls, directly or through an interface, is a helper,
// and so is what another test file of the directory spells out.
func checkDroppedCase(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	used, calledByName := usedFuncs(pass)
	aliased := aliasedSuites(pass)
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil), (*ast.TypeSpec)(nil)}, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.FuncDecl:
			checkDroppedMethod(pass, n, suites, aliased, used, calledByName)
		case *ast.TypeSpec:
			checkUnaliasedGeneric(pass, n, suites, aliased)
		}
	})
}

func checkDroppedMethod(pass *analysis.Pass, fd *ast.FuncDecl, suites map[string]*suiteInfo, aliased map[*types.TypeName]bool, used map[*types.Func]bool, calledByName map[string]bool) {
	if fd.Recv == nil || !fd.Name.IsExported() {
		return
	}
	fn, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func)
	if !ok || used[fn] || calledByName[fd.Name.Name] {
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
	isSuite := suites[typ] != nil || aliased[named.Obj()]
	switch {
	case isSuite && bare:
		report(pass, DroppedCase, funcHeader(fd),
			"%s on %s never runs: a case needs a name after %s; rename it, or give the helper a name that does not begin with %s",
			name, typ, prefix, prefix)
	case isSuite || stdlib || gotestast.IS_TEST_SUITE.MatchString(typ) || isTestifySuite(pass, typ):
	case !bare && !isCaseMethodName(name):
	case !inTestFile(pass, fd.Pos()) || spelledOutside(pass, fd.Pos(), "."+name+"("):
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
	if !ok || !hasCaseMethod(named) || spelledOutside(pass, ts.Pos(), ts.Name.Name+"[") {
		return
	}
	report(pass, DroppedCase, span{ts.Name.Pos(), ts.Name.End()},
		"generic suite %s never runs: no alias instantiates it; declare one (type …TestSuite = %s[…]), or delete it",
		ts.Name.Name, ts.Name.Name)
}

// usedFuncs returns the functions and methods the package refers to, and the
// names of interface methods it calls: such a call may reach any method of
// that name.
func usedFuncs(pass *analysis.Pass) (map[*types.Func]bool, map[string]bool) {
	used := map[*types.Func]bool{}
	byName := map[string]bool{}
	for _, obj := range pass.TypesInfo.Uses {
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		if recv := fn.Signature().Recv(); recv != nil && types.IsInterface(recv.Type()) {
			byName[fn.Name()] = true
			continue
		}
		used[fn.Origin()] = true
	}
	return used, byName
}

// aliasedSuites are the types an alias whose name ends in TestSuite names:
// the generic a suite alias instantiates, or a type it runs under another name.
func aliasedSuites(pass *analysis.Pass) map[*types.TypeName]bool {
	out := map[*types.TypeName]bool{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Assign.IsValid() || !gotestast.IS_TEST_SUITE.MatchString(ts.Name.Name) {
					continue
				}
				if named, ok := types.Unalias(pass.TypesInfo.TypeOf(ts.Type)).(*types.Named); ok {
					out[named.Origin().Obj()] = true
				}
			}
		}
	}
	return out
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

// spelledOutside reports whether a _test.go file of pos's directory that is
// not in this pass, such as the external test package's, contains needle.
func spelledOutside(pass *analysis.Pass, pos token.Pos, needle string) bool {
	dir := filepath.Dir(pass.Fset.File(pos).Name())
	own := map[string]bool{}
	for _, f := range pass.Files {
		own[filepath.Clean(pass.Fset.File(f.Pos()).Name())] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") || own[path] || about.PSuiteRegex.MatchString(e.Name()) {
			continue
		}
		if src, err := os.ReadFile(path); err == nil && strings.Contains(string(src), needle) { //nolint:gosec // a file of the package's own directory
			return true
		}
	}
	return false
}
