package gotestgen

import (
	"go/ast"
	"go/token"

	goinspect "golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
)

type CollectorError struct {
	Err error
	Pos token.Pos
}

type CollectorResult struct {
	Suites   gotestast.TestSuiteSpecSet
	Fixtures []*gotestast.FixtureSpec
	Errs     []CollectorError
}

type collector struct{}

// NewCollector returns a new collector instance for use outside this package.
func NewCollector() collector {
	return collector{}
}

func (collector) CollectSuiteSpecs(pkg *packages.Package) CollectorResult {
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

	// find fixtures
	insp.Preorder([]ast.Node{(*ast.GenDecl)(nil)}, func(n ast.Node) {
		f, err := gotestast.DetermineFixture(n, pkg)
		if err != nil {
			errs = append(errs, CollectorError{Err: err})
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

	// find suite methods
	for _, s := range suites {
		insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
			pos, err := gotestast.DetermineTestSuiteHarness(n, pkg, s)
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
			errs = append(errs, CollectorError{Err: err, Pos: s.Pos()})
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

	// Fixture embedding and validation are handled by the resolver (resolver.go),
	// which walks the type graph recursively and supports cross-package fixtures.

	return CollectorResult{Suites: suites, Fixtures: fixtures}
}

type SpecOutcome struct {
	EffectiveTestSuites gotestast.TestSuiteSpecSet
	SkippedTestSuites   gotestast.SkippedTestSuites // skipped == unfocused + excluded
	SkippedTestCases    gotestast.SkippedTestCases  // skipped == unfocused + excluded
	Fixtures            []*gotestast.FixtureSpec
}

func (collector) ApplyTestSuiteSpecs(result CollectorResult) (spec SpecOutcome, _ error) {
	suites, skippedTestSuites, skippedTestCases := result.Suites.ReduceToEffectiveSet()

	// TODO: sort all by name

	return SpecOutcome{
		EffectiveTestSuites: suites,
		SkippedTestSuites:   skippedTestSuites,
		SkippedTestCases:    skippedTestCases,
		Fixtures:            result.Fixtures,
	}, nil
}

