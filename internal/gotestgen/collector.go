package gotestgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	goinspect "golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/protocol"
)

type CollectorError struct {
	Err error
	Pos token.Pos
}

type CollectorResult struct {
	Suites   gotestast.TestSuiteSpecSet
	Fixtures []*gotestast.FixtureSpec
	Errs     []CollectorError
	// Warnings never stop generation; discover reports them.
	Warnings []gotestast.Warning
}

type collector struct {
	sources *sourceLoader // loads a dependency's source when its syntax is not at hand
}

// NewCollector returns a new collector instance for use outside this package.
func NewCollector() collector {
	return collector{}
}

func (c collector) CollectSuiteSpecs(pkg *packages.Package) CollectorResult {
	if pkg == nil {
		return CollectorResult{}
	}
	insp := goinspect.New(pkg.Syntax)

	var suites gotestast.TestSuiteSpecSet
	var fixtures []*gotestast.FixtureSpec
	var errs []CollectorError

	// find suites
	insp.Preorder([]ast.Node{(*ast.GenDecl)(nil)}, func(n ast.Node) {
		s, pos, err := gotestast.DetermineTestSuite(n, pkg)
		if err != nil {
			errs = append(errs, CollectorError{Err: err, Pos: pos})
			return
		}
		if s == nil {
			return
		}
		suites = append(suites, s)
	})
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}
	// the harness compiles in the package's scope
	if len(suites) > 0 {
		if pos, err := gotestast.PredeclaredClash(pkg); err != nil {
			return CollectorResult{Errs: []CollectorError{{Err: err, Pos: pos}}}
		}
	}

	// find fixtures: a package fixture only when a suite reaches it, a shared
	// fixture always, since suites in other packages bind it
	reached := gotestast.ReachedFixtures(pkg, suites)
	wanted := func(name string) bool {
		return reached[name] || strings.HasSuffix(name, protocol.SuffixSharedFixture)
	}
	insp.Preorder([]ast.Node{(*ast.GenDecl)(nil)}, func(n ast.Node) {
		f, err := gotestast.DetermineFixtureOf(n, pkg, wanted)
		if err != nil {
			errs = append(errs, CollectorError{Err: err, Pos: gotestast.PosOf(gotestast.At(n.Pos(), err))})
			return
		}
		if f == nil {
			return
		}
		fixtures = append(fixtures, f)
	})
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	// a suite or fixture composes through named fields; embedding and nesting
	// are refused where they are written
	for _, s := range suites {
		if pos, err := gotestast.ValidateSuiteShape(s); err != nil {
			errs = append(errs, CollectorError{Err: err, Pos: pos})
		}
	}
	for _, f := range fixtures {
		if pos, err := gotestast.ValidateFixtureShape(f); err != nil {
			errs = append(errs, CollectorError{Err: err, Pos: pos})
		}
	}
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	// find suite methods; an alias of a generic suite from another package
	// runs the methods that package declares
	for _, s := range suites {
		declPkg, err := c.declaringPackage(pkg, s)
		if err != nil {
			errs = append(errs, CollectorError{Err: err, Pos: s.Pos()})
			continue
		}
		walk := insp
		if declPkg == nil {
			declPkg = pkg
		} else {
			walk = goinspect.New(declPkg.Syntax)
		}
		walk.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
			pos, err := gotestast.DetermineTestSuiteHarness(n, declPkg, s)
			if err != nil {
				errs = append(errs, CollectorError{Err: err, Pos: pos})
			}
		})
	}
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	// validate context consistency
	for _, s := range suites {
		if err := gotestast.ValidateContextConsistency(s); err != nil {
			errs = append(errs, CollectorError{Err: err, Pos: gotestast.PosOf(gotestast.At(s.Pos(), err))})
		}
	}
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	// find fixture methods
	for _, f := range fixtures {
		insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
			pos, err := gotestast.DetermineFixtureHarness(n, pkg, f)
			if err != nil {
				errs = append(errs, CollectorError{Err: err, Pos: pos})
			}
		})
	}
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	// validate fixture consistency (e.g. Hydrate/Dehydrate pairing)
	for _, f := range fixtures {
		if pos, err := gotestast.ValidateFixtureConsistency(f); err != nil {
			errs = append(errs, CollectorError{Err: err, Pos: pos})
		}
	}
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	// Fixture embedding and validation are handled by binding (binding.go),
	// which walks the type graph recursively and supports cross-package fixtures.

	return CollectorResult{Suites: suites, Fixtures: fixtures, Warnings: gotestast.HarnessWarnings(pkg, suites)}
}

type SpecOutcome struct {
	EffectiveTestSuites gotestast.TestSuiteSpecSet
	SkippedTestSuites   gotestast.SkippedTestSuites // skipped == unfocused + excluded
	SkippedTestCases    gotestast.SkippedTestCases  // skipped == unfocused + excluded
	Fixtures            []*gotestast.FixtureSpec
}

func (c collector) ApplyTestSuiteSpecs(result CollectorResult) (spec SpecOutcome, _ error) { //nolint:gocritic // hugeParam: stable API
	return c.applyInRun(result, result.Suites.Focuses())
}

// applyInRun is ApplyTestSuiteSpecs inside a run that holds a focused item
// when focusRun is set, in this package or any other.
func (collector) applyInRun(result CollectorResult, focusRun bool) (spec SpecOutcome, _ error) { //nolint:gocritic // hugeParam: stable API
	suites, skippedTestSuites, skippedTestCases := result.Suites.ReduceInRun(focusRun)

	// TODO: sort all by name

	return SpecOutcome{
		EffectiveTestSuites: suites,
		SkippedTestSuites:   skippedTestSuites,
		SkippedTestCases:    skippedTestCases,
		Fixtures:            result.Fixtures,
	}, nil
}

// declaringPackage returns the package, with syntax, that declares the generic
// suite s aliases, or nil when s is no alias or the generic is declared in pkg.
func (c collector) declaringPackage(pkg *packages.Package, s *gotestast.TestSuiteSpec) (*packages.Package, error) {
	if !s.IsGenericAlias() || pkg.Types == nil {
		return nil, nil
	}
	obj := pkg.Types.Scope().Lookup(s.Identifier())
	if obj == nil {
		return nil, nil
	}
	named, ok := types.Unalias(obj.Type()).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() == pkg.PkgPath {
		return nil, nil
	}
	path := named.Obj().Pkg().Path()
	if imp := pkg.Imports[path]; imp != nil && len(imp.Syntax) > 0 && imp.TypesInfo != nil {
		return imp, nil
	}
	sources := c.sources
	if sources == nil { // a collector made outside a package load
		sources = newSourceLoader(nil, pkg.Fset)
	}
	if src := sources.load(path); src != nil {
		return src, nil
	}
	return nil, fmt.Errorf("suite %s aliases %s.%s, whose source cannot be loaded", s.Identifier(), named.Obj().Pkg().Name(), named.Obj().Name())
}
