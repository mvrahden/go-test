package gotestgen

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// loadTestPkg parses source code into a type-checked *packages.Package.
// It resolves imports using the default Go importer so that references
// to gotest.T and error work correctly.
func loadTestPkg(t *testing.T, src string) *packages.Package {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	gotest.NoError(t, err)

	conf := types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
	}
	info := &types.Info{
		Types:  make(map[ast.Expr]types.TypeAndValue),
		Defs:   make(map[*ast.Ident]types.Object),
		Uses:   make(map[*ast.Ident]types.Object),
		Scopes: make(map[ast.Node]*types.Scope),
	}
	tpkg, err := conf.Check("testpkg", fset, []*ast.File{f}, info)
	gotest.NoError(t, err)

	return &packages.Package{
		Name:      tpkg.Name(),
		Fset:      fset,
		Syntax:    []*ast.File{f},
		TypesInfo: info,
		Types:     tpkg,
	}
}

// loadTestPkgWithGotest loads a package that imports gotest.T using the full
// packages.Load machinery. This writes source to a temp directory and loads it.
func loadTestPkgWithGotest(t *testing.T, src string) *packages.Package {
	t.Helper()

	// Find the module root for replace directive
	modRoot, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)

	dir := t.TempDir()
	err = os.WriteFile(filepath.Join(dir, "test.go"), []byte(src), 0644)
	gotest.NoError(t, err)

	goMod := `module testpkg

go 1.24

require github.com/mvrahden/go-test v0.0.0

replace github.com/mvrahden/go-test => ` + modRoot + `
`
	err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644)
	gotest.NoError(t, err)

	pkgs, err := packages.Load(&packages.Config{
		Mode: packageEvalMode,
		Dir:  dir,
	}, ".")
	gotest.NoError(t, err)
	gotest.True(t, len(pkgs) > 0, "expected at least one package loaded")

	pkg := pkgs[0]
	gotest.True(t, len(pkg.Errors) == 0, "expected no package errors, got: %v", pkg.Errors)
	return pkg
}

// --- Fixture collection tests ---

func TestCollector_FixtureCollection_PackageFixture(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.Equal(t, gotestast.PackageFixture, result.Fixtures[0].Kind)
	gotest.Equal(t, "DBFixture", result.Fixtures[0].Identifier())
	gotest.True(t, result.Fixtures[0].BeforeAll != nil, "expected BeforeAll to be set")
	gotest.True(t, result.Fixtures[0].AfterAll == nil, "expected AfterAll to be nil")
}

func TestCollector_FixtureCollection_PackageFixtureAllMethods(t *testing.T) {
	src := `package testpkg

import "context"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *DBFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *DBFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *DBFixture) AfterEach(ctx context.Context) error  { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))

	fix := result.Fixtures[0]
	gotest.True(t, fix.BeforeAll != nil, "expected BeforeAll")
	gotest.True(t, fix.AfterAll != nil, "expected AfterAll")
	gotest.True(t, fix.BeforeEach != nil, "expected BeforeEach")
	gotest.True(t, fix.AfterEach != nil, "expected AfterEach")
}

func TestCollector_FixtureCollection_SharedFixture(t *testing.T) {
	src := `package testpkg

type RedisSharedFixture struct {
	Addr string
}

func (f *RedisSharedFixture) BeforeAll() error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.Equal(t, gotestast.SharedFixture, result.Fixtures[0].Kind)
	gotest.True(t, result.Fixtures[0].BeforeAll != nil, "expected BeforeAll to be set")
}

func TestCollector_FixtureCollection_SharedFixtureWithAfterAll(t *testing.T) {
	src := `package testpkg

type RedisSharedFixture struct {
	Addr string
}

func (f *RedisSharedFixture) BeforeAll() error { return nil }
func (f *RedisSharedFixture) AfterAll() error  { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Fixtures))

	fix := result.Fixtures[0]
	gotest.True(t, fix.BeforeAll != nil, "expected BeforeAll")
	gotest.True(t, fix.AfterAll != nil, "expected AfterAll")
}

// --- Fixture embedding in test suites ---

func TestCollector_FixtureEmbeddingInTestSuite(t *testing.T) {
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type MyTestSuite struct {
	*DBFixture
}

func (s *MyTestSuite) TestSomething(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.Equal(t, 1, len(result.Fixtures))
	gotest.True(t, result.Suites[0].Fixture() != nil, "expected fixture to be linked to suite")
	gotest.Equal(t, "DBFixture", result.Suites[0].Fixture().Identifier())
}

func TestCollector_NoFixtureEmbedding(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyTestSuite struct{}

func (s *MyTestSuite) TestSomething(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.True(t, result.Suites[0].Fixture() == nil, "expected no fixture")
}

// --- Fixture-to-fixture embedding ---

func TestCollector_FixtureToFixtureEmbedding(t *testing.T) {
	src := `package testpkg

import "context"

type BaseFixture struct{}

func (f *BaseFixture) BeforeAll(ctx context.Context) error { return nil }

type DBFixture struct {
	*BaseFixture
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	// Note: this will error because there are 2 package fixtures
	// For this test we just verify collection worked up to that point
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 2, len(result.Fixtures))

	// Find the DBFixture (it embeds BaseFixture)
	var dbFix *gotestast.FixtureSpec
	for _, f := range result.Fixtures {
		if f.Identifier() == "DBFixture" {
			dbFix = f
			break
		}
	}
	gotest.True(t, dbFix != nil, "expected to find DBFixture")
	gotest.True(t, dbFix.ParentFixture != nil, "expected parent fixture to be set")
	gotest.Equal(t, "BaseFixture", dbFix.ParentFixture.Identifier())
}

// --- Validation: Multiple package fixtures ---

func TestValidation_MultipleRootPackageFixtures(t *testing.T) {
	fixtures := []*gotestast.FixtureSpec{
		makeFixtureSpec("Fix1", gotestast.PackageFixture, true),
		makeFixtureSpec("Fix2", gotestast.PackageFixture, true),
	}
	err := validateFixtures(fixtures)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "at most one root package fixture")
}

func TestValidation_SinglePackageFixture_OK(t *testing.T) {
	fixtures := []*gotestast.FixtureSpec{
		makeFixtureSpec("Fix1", gotestast.PackageFixture, true),
	}
	err := validateFixtures(fixtures)
	gotest.NoError(t, err)
}

func TestValidation_NestedPackageFixtures_OK(t *testing.T) {
	root := makeFixtureSpec("Root", gotestast.PackageFixture, true)
	child := makeFixtureSpec("Child", gotestast.PackageFixture, true)
	child.ParentFixture = root
	fixtures := []*gotestast.FixtureSpec{root, child}
	err := validateFixtures(fixtures)
	gotest.NoError(t, err)
}

// --- Validation: Missing BeforeAll ---

func TestValidation_PackageFixtureMissingBeforeAll(t *testing.T) {
	fixtures := []*gotestast.FixtureSpec{
		makeFixtureSpec("Fix1", gotestast.PackageFixture, false),
	}
	err := validateFixtures(fixtures)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "must have a BeforeAll")
}

func TestValidation_SharedFixtureMissingBeforeAll(t *testing.T) {
	fixtures := []*gotestast.FixtureSpec{
		makeFixtureSpec("Fix1", gotestast.SharedFixture, false),
	}
	err := validateFixtures(fixtures)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "must have a BeforeAll")
}

// --- Validation: Fixture cycles ---

func TestValidation_FixtureCycle(t *testing.T) {
	a := makeFixtureSpec("A", gotestast.PackageFixture, true)
	b := makeFixtureSpec("B", gotestast.PackageFixture, true)
	a.ParentFixture = b
	b.ParentFixture = a

	err := validateFixtureCycles([]*gotestast.FixtureSpec{a, b})
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "cycle detected")
}

func TestValidation_NoCycle(t *testing.T) {
	a := makeFixtureSpec("A", gotestast.PackageFixture, true)
	b := makeFixtureSpec("B", gotestast.PackageFixture, true)
	b.ParentFixture = a

	err := validateFixtureCycles([]*gotestast.FixtureSpec{a, b})
	gotest.NoError(t, err)
}

func TestValidation_SelfCycle(t *testing.T) {
	a := makeFixtureSpec("A", gotestast.PackageFixture, true)
	a.ParentFixture = a

	err := validateFixtureCycles([]*gotestast.FixtureSpec{a})
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "cycle detected")
}

// --- Validation: shared fixture wrong signature ---

func TestCollector_SharedFixture_BeforeEachDisallowed(t *testing.T) {
	src := `package testpkg

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll() error    { return nil }
func (f *RedisSharedFixture) BeforeEach() error   { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for BeforeEach on shared fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "must not have BeforeEach")
}

func TestCollector_SharedFixture_AfterEachDisallowed(t *testing.T) {
	src := `package testpkg

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll() error  { return nil }
func (f *RedisSharedFixture) AfterEach() error  { return nil }
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for AfterEach on shared fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "must not have AfterEach")
}

func TestCollector_SharedFixture_WrongBeforeAllSignature(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll(t *gotest.T) {} // wrong: should be () error
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for wrong BeforeAll signature on shared fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported signature")
}

// --- Validation: suite embeds multiple fixtures ---

func TestCollector_SuiteEmbedsMultipleFixtures(t *testing.T) {
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type OneFixture struct{}
func (f *OneFixture) BeforeAll(ctx context.Context) error { return nil }

type TwoFixture struct{}
func (f *TwoFixture) BeforeAll(ctx context.Context) error { return nil }

type MyTestSuite struct {
	*OneFixture
	*TwoFixture
}

func (s *MyTestSuite) TestSomething(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	// There are 2 package fixtures which would fail validation in ApplyTestSuiteSpecs,
	// but the embedding check happens first in CollectSuiteSpecs
	gotest.True(t, len(result.Errs) > 0, "expected error for multiple fixture embeddings")
	gotest.Contains(t, result.Errs[0].Err.Error(), "embeds multiple fixtures")
}

// --- Validation: ApplyTestSuiteSpecs ---

func TestApplyTestSuiteSpecs_MultipleRootPackageFixturesError(t *testing.T) {
	fixtures := []*gotestast.FixtureSpec{
		makeFixtureSpec("Fix1", gotestast.PackageFixture, true),
		makeFixtureSpec("Fix2", gotestast.PackageFixture, true),
	}
	c := collector{}
	_, err := c.ApplyTestSuiteSpecs(CollectorResult{Fixtures: fixtures})
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "at most one root package fixture")
}

func TestApplyTestSuiteSpecs_CycleError(t *testing.T) {
	a := makeFixtureSpec("A", gotestast.PackageFixture, true)
	b := makeFixtureSpec("B", gotestast.SharedFixture, true)
	a.ParentFixture = b
	b.ParentFixture = a

	c := collector{}
	_, err := c.ApplyTestSuiteSpecs(CollectorResult{Fixtures: []*gotestast.FixtureSpec{a, b}})
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "cycle detected")
}

func TestApplyTestSuiteSpecs_MissingBeforeAllError(t *testing.T) {
	c := collector{}
	_, err := c.ApplyTestSuiteSpecs(CollectorResult{
		Fixtures: []*gotestast.FixtureSpec{
			makeFixtureSpec("Fix1", gotestast.PackageFixture, false),
		},
	})
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "must have a BeforeAll")
}

func TestApplyTestSuiteSpecs_OK(t *testing.T) {
	c := collector{}
	spec, err := c.ApplyTestSuiteSpecs(CollectorResult{
		Fixtures: []*gotestast.FixtureSpec{
			makeFixtureSpec("Fix1", gotestast.PackageFixture, true),
		},
	})
	gotest.NoError(t, err)
	gotest.True(t, spec.EffectiveTestSuites == nil, "expected no suites")
}

// --- Package fixture wrong method signature ---

func TestCollector_PackageFixture_WrongBeforeAllSignature(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type DBFixture struct{}

func (f *DBFixture) BeforeAll(t *gotest.T) {} // wrong: should be (ctx context.Context) error
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.True(t, len(result.Errs) > 0, "expected error for wrong BeforeAll signature on package fixture")
	gotest.Contains(t, result.Errs[0].Err.Error(), "unsupported signature")
}

// --- Nil package ---

func TestCollector_NilPackage(t *testing.T) {
	c := collector{}
	result := c.CollectSuiteSpecs(nil)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.True(t, result.Suites == nil, "expected nil suites")
	gotest.True(t, result.Fixtures == nil, "expected nil fixtures")
}

// --- helpers ---

// makeFixtureSpec creates a minimal FixtureSpec for validation testing.
// It uses gotestast.NewFixtureSpecForTest which we need to add.
func makeFixtureSpec(name string, kind gotestast.FixtureKind, hasBeforeAll bool) *gotestast.FixtureSpec {
	f := gotestast.NewFixtureSpecForTest(name, kind)
	if hasBeforeAll {
		f.BeforeAll = types.NewFunc(token.NoPos, nil, "BeforeAll", nil)
	}
	return f
}
