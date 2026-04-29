package gotestgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

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

	// detect fixture embedding in test suites (only package fixtures, not shared fixtures)
	for _, s := range suites {
		embedded := findEmbeddedFixtures(s.StructType(), fixtures)
		var pkgFixtures []*gotestast.FixtureSpec
		for _, e := range embedded {
			if e.Kind == gotestast.PackageFixture {
				pkgFixtures = append(pkgFixtures, e)
			}
		}
		if len(pkgFixtures) > 1 {
			errs = append(errs, CollectorError{
				Err: fmt.Errorf("test suite %q embeds multiple fixtures; at most one is allowed", s.Identifier()),
			})
			continue
		}
		if len(pkgFixtures) == 1 {
			s.SetFixture(pkgFixtures[0])
		}
	}
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	// detect fixture-to-fixture embedding (parent fixture, skip shared fixtures)
	for _, f := range fixtures {
		embedded := findEmbeddedFixtures(f.StructType(), fixtures)
		var parents []*gotestast.FixtureSpec
		for _, e := range embedded {
			if e != f && e.Kind != gotestast.SharedFixture {
				parents = append(parents, e)
			}
		}
		if len(parents) > 1 {
			errs = append(errs, CollectorError{
				Err: fmt.Errorf("fixture %q embeds multiple fixtures; at most one parent is allowed", f.Identifier()),
			})
			continue
		}
		if len(parents) == 1 {
			f.ParentFixture = parents[0]
		}
	}
	if len(errs) > 0 {
		return CollectorResult{Errs: errs}
	}

	return CollectorResult{Suites: suites, Fixtures: fixtures}
}

// findEmbeddedFixtures inspects a struct type for anonymous (embedded) pointer
// fields that match any of the given fixture types.
func findEmbeddedFixtures(typ *types.Struct, fixtures []*gotestast.FixtureSpec) []*gotestast.FixtureSpec {
	if typ == nil {
		return nil
	}
	var found []*gotestast.FixtureSpec
	for i := 0; i < typ.NumFields(); i++ {
		field := typ.Field(i)
		if !field.Anonymous() {
			continue
		}
		ptr, ok := field.Type().(*types.Pointer)
		if !ok {
			continue
		}
		named, ok := ptr.Elem().(*types.Named)
		if !ok {
			continue
		}
		for _, f := range fixtures {
			if named.Obj().Name() == f.Identifier() {
				found = append(found, f)
				break
			}
		}
	}
	return found
}

type SpecOutcome struct {
	EffectiveTestSuites gotestast.TestSuiteSpecSet
	SkippedTestSuites   gotestast.SkippedTestSuites // skipped == unfocused + excluded
	SkippedTestCases    gotestast.SkippedTestCases  // skipped == unfocused + excluded
	Fixtures            []*gotestast.FixtureSpec
}

func (collector) ApplyTestSuiteSpecs(result CollectorResult) (spec SpecOutcome, _ error) {
	// Validate fixture constraints
	if err := validateFixtures(result.Fixtures); err != nil {
		return SpecOutcome{}, err
	}

	// Validate fixture cycle constraints
	if err := validateFixtureCycles(result.Fixtures); err != nil {
		return SpecOutcome{}, err
	}

	suites, skippedTestSuites, skippedTestCases := result.Suites.ReduceToEffectiveSet()

	// TODO: sort all by name

	return SpecOutcome{
		EffectiveTestSuites: suites,
		SkippedTestSuites:   skippedTestSuites,
		SkippedTestCases:    skippedTestCases,
		Fixtures:            result.Fixtures,
	}, nil
}

// validateFixtures validates fixture constraints:
// - At most one root package fixture per package (root = no parent fixture)
// - Package fixture MUST have BeforeAll method
// - Shared fixture MUST have BeforeAll method (with () error signature)
func validateFixtures(fixtures []*gotestast.FixtureSpec) error {
	var rootPkgFixtureCount int
	for _, f := range fixtures {
		if f.Kind == gotestast.PackageFixture && f.ParentFixture == nil {
			rootPkgFixtureCount++
		}
	}
	if rootPkgFixtureCount > 1 {
		return fmt.Errorf("at most one root package fixture (*Fixture without parent) is allowed per package, found %d", rootPkgFixtureCount)
	}

	for _, f := range fixtures {
		switch f.Kind {
		case gotestast.PackageFixture:
			if f.BeforeAll == nil {
				return fmt.Errorf("package fixture %q must have a BeforeAll(ctx context.Context) error method", f.Identifier())
			}
		case gotestast.SharedFixture:
			if f.BeforeAll == nil {
				return fmt.Errorf("shared fixture %q must have a BeforeAll(ctx context.Context) error method", f.Identifier())
			}
		}
	}

	return nil
}

// validateFixtureCycles detects cycles in fixture embedding chains.
func validateFixtureCycles(fixtures []*gotestast.FixtureSpec) error {
	for _, f := range fixtures {
		visited := map[string]bool{}
		current := f
		for current != nil {
			if visited[current.Identifier()] {
				return fmt.Errorf("cycle detected in fixture embedding: %q is part of a cycle", f.Identifier())
			}
			visited[current.Identifier()] = true
			current = current.ParentFixture
		}
	}
	return nil
}
