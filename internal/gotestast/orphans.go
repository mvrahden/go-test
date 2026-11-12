package gotestast

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/typeclass"
	"golang.org/x/tools/go/packages"
)

// Orphan is a declaration that reads as part of a suite and that no run reaches.
type Orphan struct {
	Pos token.Pos
	Msg string
}

// caseKind is a case prefix with its handle and the wording of each.
type caseKind struct {
	prefix string
	handle typeclass.Handle
	param  string
	noun   string
}

var casePrefixes = []caseKind{
	{"Test", typeclass.TestHandle, "*gotest.T", "test"},
	{protocol.PrefixBenchmark, typeclass.BenchHandle, "*gotest.B", "benchmark"},
	{protocol.PrefixFuzz, typeclass.FuzzHandle, "*gotest.F", "fuzz target"},
}

// Orphans lists them for the test variants of one package (internal, external;
// a nil variant is skipped), in source order. It reads _test.go files only.
func Orphans(variants ...*packages.Package) []Orphan {
	suiteKeys := map[string]bool{}
	called := map[string]bool{}
	calledByName := map[string]bool{}
	// test files the build constraints leave out may call what this build
	// cannot see called
	selectedLeftOut, namedLeftOut := leftOutNames(variants)
	for _, pkg := range variants {
		if pkg == nil || pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, d := range file.Decls {
				if s, _, _ := DetermineTestSuite(d, pkg); s != nil {
					suiteKeys[s.recvKey] = true
				}
			}
		}
		for _, obj := range pkg.TypesInfo.Uses {
			fn, ok := obj.(*types.Func)
			if !ok {
				continue
			}
			recv := fn.Signature().Recv()
			if recv == nil {
				continue
			}
			if types.IsInterface(recv.Type()) {
				calledByName[fn.Name()] = true
				continue
			}
			if n := namedBehind(recv.Type()); n != nil {
				called[typeKey(n)+"."+fn.Name()] = true
			}
		}
	}

	type found struct {
		orphan Orphan
		more   int
	}
	var out []*found
	byType := map[string]*found{}
	for _, pkg := range variants {
		if pkg == nil || pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			name := pkg.Fset.File(file.Pos()).Name()
			if !strings.HasSuffix(name, "_test.go") || about.PSuiteRegex.MatchString(name) {
				continue
			}
			for _, d := range file.Decls {
				if gd, ok := d.(*ast.GenDecl); ok {
					if o, ok := uninstantiated(gd, pkg, suiteKeys); ok && !namedLeftOut[o.name] {
						out = append(out, &found{orphan: o.Orphan})
					}
					continue
				}
				decl, ok := d.(*ast.FuncDecl)
				if !ok || decl.Recv == nil || !decl.Name.IsExported() {
					continue
				}
				fn, ok := pkg.TypesInfo.ObjectOf(decl.Name).(*types.Func)
				if !ok || fn.Signature().Recv() == nil {
					continue
				}
				n := namedBehind(fn.Signature().Recv().Type())
				if n == nil {
					continue
				}
				key := typeKey(n)
				if IS_TEST_SUITE.MatchString(n.Obj().Name()) || suiteKeys[key] {
					continue
				}
				method := decl.Name.Name
				c, ok := caseOf(method, fn.Signature())
				if !ok || called[key+"."+method] || calledByName[method] || selectedLeftOut[method] {
					continue
				}
				if f := byType[key]; f != nil {
					f.more++
					continue
				}
				typ := n.Obj().Name()
				f := &found{orphan: Orphan{
					Pos: decl.Name.Pos(),
					Msg: fmt.Sprintf("%s.%s takes %s, but %s is not a test suite (its name must end in TestSuite): the %s never runs", typ, method, c.param, typ, c.noun),
				}}
				byType[key] = f
				out = append(out, f)
			}
		}
	}

	orphans := make([]Orphan, 0, len(out))
	for _, f := range out {
		if f.more > 0 {
			f.orphan.Msg += fmt.Sprintf(", nor do %d more of its methods", f.more)
		}
		orphans = append(orphans, f.orphan)
	}
	sort.SliceStable(orphans, func(i, j int) bool { return orphans[i].Pos < orphans[j].Pos })
	return orphans
}

// uninstantiated refuses a generic suite no alias named as a suite
// instantiates.
func uninstantiated(gd *ast.GenDecl, pkg *packages.Package, suiteKeys map[string]bool) (genericOrphan, bool) {
	if gd.Tok != token.TYPE || len(gd.Specs) != 1 {
		return genericOrphan{}, false
	}
	ts, ok := gd.Specs[0].(*ast.TypeSpec)
	if !ok || ts.TypeParams == nil || ts.TypeParams.NumFields() == 0 || !IS_TEST_SUITE.MatchString(ts.Name.Name) {
		return genericOrphan{}, false
	}
	obj, ok := pkg.TypesInfo.Defs[ts.Name].(*types.TypeName)
	if !ok {
		return genericOrphan{}, false
	}
	named, ok := obj.Type().(*types.Named)
	if !ok || !declaresCase(named) || suiteKeys[typeKey(named)] {
		return genericOrphan{}, false
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return genericOrphan{}, false
	}
	name := ts.Name.Name
	return genericOrphan{Orphan{
		Pos: ts.Name.Pos(),
		Msg: fmt.Sprintf("generic suite %s has no instantiation, so its cases never run; declare one as an alias whose name ends in TestSuite (type XTestSuite = %s[...])", name, name),
	}, name}, true
}

// genericOrphan is an uninstantiated generic suite and its name.
type genericOrphan struct {
	Orphan
	name string
}

// leftOutNames reads the test files the build constraints leave out of the
// variants, syntax only: the names they select (x.Name) and every name they
// spell. A file that does not parse names nothing.
func leftOutNames(variants []*packages.Package) (selected, named map[string]bool) {
	selected, named = map[string]bool{}, map[string]bool{}
	seen := map[string]bool{}
	fset := token.NewFileSet()
	for _, pkg := range variants {
		if pkg == nil {
			continue
		}
		for _, path := range pkg.IgnoredFiles {
			if !strings.HasSuffix(path, "_test.go") || seen[path] {
				continue
			}
			seen[path] = true
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.SelectorExpr:
					selected[n.Sel.Name] = true
				case *ast.Ident:
					named[n.Name] = true
				}
				return true
			})
		}
	}
	return selected, named
}

// caseOf returns the case a method named as one is, when it takes that case's
// gotest handle first.
func caseOf(name string, sig *types.Signature) (c caseKind, ok bool) {
	stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
	if !IS_TEST_CASE.MatchString(name) && !IS_BENCHMARK.MatchString(name) && !IS_FUZZ.MatchString(name) && !isBareCaseName(name) {
		return c, false
	}
	if sig.Params().Len() == 0 {
		return c, false
	}
	h, stdlib := typeclass.CaseHandle(sig.Params().At(0).Type())
	if stdlib {
		return c, false
	}
	for _, p := range casePrefixes {
		if strings.HasPrefix(stripped, p.prefix) {
			return p, h == p.handle
		}
	}
	return c, false
}

// isBareCaseName reports whether name, an X_ or F_ marker aside, is a case
// prefix alone: Test, Benchmark or Fuzz.
func isBareCaseName(name string) bool {
	stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
	return stripped == "Test" || stripped == protocol.PrefixBenchmark || stripped == protocol.PrefixFuzz
}
