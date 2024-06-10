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
	Suites gotestast.TestSuiteSpecSet
	Errs   []CollectorError
}

type collector struct{}

func (collector) CollectSuiteSpecs(pkg *packages.Package) CollectorResult {
	if pkg == nil {
		return CollectorResult{}
	}
	insp := goinspect.New(pkg.Syntax)

	var suites gotestast.TestSuiteSpecSet
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

	// find methods
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

	// add suites to result
	return CollectorResult{Suites: suites}
}

type SpecOutcome struct {
	EffectiveTestSuites gotestast.TestSuiteSpecSet
	SkippedTestSuites   gotestast.SkippedTestSuites // skipped == unfocused + excluded
	SkippedTestCases    gotestast.SkippedTestCases  // skipped == unfocused + excluded
}

func (collector) ApplyGoTestSpecs(suites gotestast.TestSuiteSpecSet) (spec SpecOutcome, _ error) {
	suites, skippedTestSuites, skippedTestCases := suites.ReduceToEffectiveSet()

	// TODO: sort all by name

	return SpecOutcome{
		EffectiveTestSuites: suites,
		SkippedTestSuites:   skippedTestSuites,
		SkippedTestCases:    skippedTestCases, // map of both
	}, nil
}
