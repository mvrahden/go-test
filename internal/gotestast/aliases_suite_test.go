package gotestast_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// loadNamedSource type-checks src as the file filename and returns the
// package with its type declarations and functions.
func loadNamedSource(t *testing.T, filename, src string) fixtureWithMethods {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	gotest.NoError(t, err)

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Importer: importer.Default()}
	tpkg, err := conf.Check("testpkg", fset, []*ast.File{f}, info)
	gotest.NoError(t, err)

	fm := fixtureWithMethods{pkg: &packages.Package{
		Name: tpkg.Name(), PkgPath: tpkg.Path(), Fset: fset, TypesInfo: info, Syntax: []*ast.File{f},
	}}
	for _, d := range f.Decls {
		switch dd := d.(type) {
		case *ast.GenDecl:
			if dd.Tok == token.TYPE {
				fm.genDecls = append(fm.genDecls, dd)
			}
		case *ast.FuncDecl:
			fm.funcDecls = append(fm.funcDecls, dd)
		}
	}
	return fm
}

// suiteNamed returns the suite declared as name in fm, failing when it is not one.
func suiteNamed(t *gotest.T, fm fixtureWithMethods, name string) *gotestast.TestSuiteSpec {
	for _, gd := range fm.genDecls {
		spec, _, err := gotestast.DetermineTestSuite(gd, fm.pkg)
		gotest.NoError(t, err)
		if spec != nil && spec.Identifier() == name {
			return spec
		}
	}
	gotest.Fail(t, "no suite "+name)
	return nil
}

// harnessErr runs every function of fm through the suite's harness and
// returns the first rejection with its position.
func harnessErr(fm fixtureWithMethods, spec *gotestast.TestSuiteSpec) (token.Pos, error) {
	for _, fd := range fm.funcDecls {
		if pos, err := gotestast.DetermineTestSuiteHarness(fd, fm.pkg, spec); err != nil {
			return pos, err
		}
	}
	return token.NoPos, nil
}

// AliasesTestSuite covers suites named through aliases, defined types over
// suites, case methods with results and shared fixtures in test files.
type AliasesTestSuite struct{}

func (s *AliasesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AliasesTestSuite) TestAliases(t *gotest.T) {
	t.When("a suite is named through an alias", func(w *gotest.T) {
		w.It("collects a method declared on a pointer to an alias of the suite", func(it *gotest.T) {
			fm := loadNamedSource(it.T(), "s_test.go", `package testpkg
import "testing"
type RealTestSuite struct{}
type Real = RealTestSuite
func (s *Real) TestViaAlias(t *testing.T) {}
`)
			spec := suiteNamed(it, fm, "RealTestSuite")
			_, err := harnessErr(fm, spec)
			gotest.NoError(it, err)
			gotest.Len(it, spec.TestCases(), 1)
		})
		w.It("collects the generic suite's methods for an alias of an alias", func(it *gotest.T) {
			fm := loadNamedSource(it.T(), "s_test.go", `package testpkg
import "testing"
type GenericTestSuite[T any] struct{}
func (s *GenericTestSuite[T]) TestZero(t *testing.T) {}
type IntTestSuite = GenericTestSuite[int]
type IntAgainTestSuite = IntTestSuite
`)
			spec := suiteNamed(it, fm, "IntAgainTestSuite")
			gotest.True(it, spec.IsGenericAlias())
			_, err := harnessErr(fm, spec)
			gotest.NoError(it, err)
			gotest.Len(it, spec.TestCases(), 1)
		})
	})

	t.When("a suite is a defined type over another suite", func(w *gotest.T) {
		w.It("is rejected at its declaration and pointed to an alias", func(it *gotest.T) {
			fm := loadNamedSource(it.T(), "s_test.go", `package testpkg
type ATestSuite struct{}
type CopyTestSuite ATestSuite
`)
			spec, pos, err := gotestast.DetermineTestSuite(fm.genDecls[1], fm.pkg)
			gotest.Zero(it, spec)
			gotest.ErrorContains(it, err, "suite CopyTestSuite is defined over suite ATestSuite and carries none of its methods; declare an alias (= ATestSuite) or a struct of its own")
			gotest.Equal(it, "s_test.go:3:6", fm.pkg.Fset.Position(pos).String())
		})
	})
}

func (s *AliasesTestSuite) TestCaseResults(t *gotest.T) {
	t.When("a case method returns a value", func(w *gotest.T) {
		for _, c := range []struct{ Name, Decl, Message string }{
			{"Test", "TestBad(t *testing.T) error", "TestBad on XTestSuite returns a value; a Test method returns nothing"},
			{"Benchmark", "BenchmarkBad(b *testing.B) int", "BenchmarkBad on XTestSuite returns a value; a Benchmark method returns nothing"},
			{"Fuzz", "FuzzBad(f *testing.F) error", "FuzzBad on XTestSuite returns a value; a Fuzz method returns nothing"},
		} {
			w.It("rejects a "+c.Name+" method at the method", func(it *gotest.T) {
				fm := loadNamedSource(it.T(), "s_test.go", `package testpkg
import "testing"
type XTestSuite struct{}
func (s *XTestSuite) `+c.Decl+` { panic("") }
`)
				spec := suiteNamed(it, fm, "XTestSuite")
				pos, err := harnessErr(fm, spec)
				gotest.ErrorContains(it, err, c.Message)
				gotest.Equal(it, "s_test.go:4:22", fm.pkg.Fset.Position(pos).String())
			})
		}
	})
}

func (s *AliasesTestSuite) TestSharedFixtureFile(t *gotest.T) {
	const src = `package testpkg
type PGSharedFixture struct{}
`
	t.When("a shared fixture is declared in a _test.go file", func(w *gotest.T) {
		w.It("is rejected at its declaration", func(it *gotest.T) {
			fm := loadNamedSource(it.T(), "pg_test.go", src)
			_, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.ErrorContains(it, err, "shared fixture PGSharedFixture is declared in a _test.go file; the setup program cannot import it, move it to a non-test file")
			gotest.Equal(it, "pg_test.go:2:6", fm.pkg.Fset.Position(gotestast.PosOf(err)).String())
		})
	})
	t.When("a shared fixture is declared in a non-test file", func(w *gotest.T) {
		w.It("is accepted", func(it *gotest.T) {
			fm := loadNamedSource(it.T(), "pg.go", src)
			spec, err := gotestast.DetermineFixture(fm.genDecls[0], fm.pkg)
			gotest.NoError(it, err)
			gotest.Equal(it, gotestast.SharedFixture, spec.Kind)
		})
	})
}
