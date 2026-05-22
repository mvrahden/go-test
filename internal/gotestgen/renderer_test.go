package gotestgen //nolint:stdlib-test

import (
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

func renderTestPkg(t *testing.T, pkg *packages.Package) (string, SpecOutcome) {
	t.Helper()
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no collection errors, got: %v", result.Errs)

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	var resolved *ResolveResult
	if len(spec.EffectiveTestSuites) > 0 {
		resolved, err = Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
		gotest.NoError(t, err)
	}

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec, resolved)
	gotest.NoError(t, err)
	return string(out), spec
}

func TestRenderer_FixtureWithChildSuite(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *DBFixture) AfterAll(ctx context.Context) error   { return nil }

type QueryTestSuite struct {
	*DBFixture
}

func (s *QueryTestSuite) BeforeEach(t *gotest.T) {}
func (s *QueryTestSuite) AfterEach(t *gotest.T)  {}
func (s *QueryTestSuite) TestInsert(t *gotest.T) {}
func (s *QueryTestSuite) TestSelect(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)
	gotest.True(t, len(output) > 0, "expected non-empty output")

	// Verify the output contains key structural elements
	gotest.True(t, strings.Contains(output, "func TestMain(m *testing.M)"), "expected TestMain")
	gotest.True(t, strings.Contains(output, "os.Exit(ƒƒ_GOTEST_main(m))"), "expected os.Exit(ƒƒ_GOTEST_main(m))")
	gotest.True(t, strings.Contains(output, "func ƒƒ_GOTEST_main(m *testing.M)"), "expected ƒƒ_GOTEST_main")
	gotest.True(t, strings.Contains(output, `"os"`), "expected os import")
	gotest.True(t, strings.Contains(output, "ƒ_DBFixture = &DBFixture{}"), "expected fixture instantiation")
	gotest.True(t, strings.Contains(output, "ƒ_DBFixture.BeforeAll(ctx)"), "expected BeforeAll call")
	gotest.True(t, strings.Contains(output, `"FAIL: DBFixture.BeforeAll failed after`), "expected BeforeAll error attribution")
	gotest.True(t, strings.Contains(output, "ƒ_DBFixture.AfterAll(ctx)"), "expected AfterAll in cleanup")
	gotest.True(t, strings.Contains(output, `"DBFixture.AfterAll failed:`), "expected AfterAll error attribution")
	gotest.True(t, strings.Contains(output, "func TestQueryTestSuite(t *testing.T)"), "expected top-level TestQueryTestSuite func")
	gotest.True(t, strings.Contains(output, "ƒƒ_GOTEST_QueryTestSuite"), "expected wrapper struct")
	gotest.True(t, strings.Contains(output, "DBFixture: ƒ_DBFixture"), "expected fixture injection")
	gotest.True(t, strings.Contains(output, `t.Run("TestInsert"`), "expected TestInsert test case")
	gotest.True(t, strings.Contains(output, `t.Run("TestSelect"`), "expected TestSelect test case")

	// Verify it does NOT contain old-style Test_DBFixture or t.Run for suites
	gotest.True(t, !strings.Contains(output, "func Test_DBFixture("), "should NOT have old-style Test_DBFixture")

	// Verify wrapper struct and lifecycle methods are at file scope (not nested in functions)
	gotest.True(t, strings.Contains(output, "type ƒƒ_GOTEST_QueryTestSuite struct"), "expected wrapper struct declaration")
	gotest.True(t, strings.Contains(output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeAll(it *gotest.T)"), "expected BeforeAll wrapper")
	gotest.True(t, strings.Contains(output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeEach(it *gotest.T)"), "expected BeforeEach wrapper")
	gotest.True(t, strings.Contains(output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) AfterEach(it *gotest.T)"), "expected AfterEach wrapper")
}

func TestRenderer_FixtureWithoutAfterAll(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SimpleFixture struct {}

func (f *SimpleFixture) BeforeAll(ctx context.Context) error { return nil }

type BasicTestSuite struct {
	*SimpleFixture
}

func (s *BasicTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// AfterAll should NOT be in the cleanup since the fixture has no AfterAll
	gotest.True(t, strings.Contains(output, "func ƒƒ_GOTEST_main(m *testing.M)"), "expected ƒƒ_GOTEST_main")
	gotest.True(t, !strings.Contains(output, "ƒ_SimpleFixture.AfterAll"), "should NOT have AfterAll call")
}

func TestRenderer_MixedFixtureBoundAndStandalone(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type AppFixture struct {}

func (f *AppFixture) BeforeAll(ctx context.Context) error { return nil }

type BoundTestSuite struct {
	*AppFixture
}

func (s *BoundTestSuite) TestBound(t *gotest.T) {}

type StandaloneTestSuite struct {}

func (s *StandaloneTestSuite) TestFree(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// Should have both fixture-bound and standalone
	gotest.True(t, strings.Contains(output, "func TestMain(m *testing.M)"), "expected TestMain for fixture")
	gotest.True(t, strings.Contains(output, "func ƒƒ_GOTEST_main(m *testing.M)"), "expected ƒƒ_GOTEST_main")
	gotest.True(t, strings.Contains(output, "func TestBoundTestSuite(t *testing.T)"), "expected top-level TestBoundTestSuite func")
	gotest.True(t, strings.Contains(output, "func TestStandaloneTestSuite(t *testing.T)"), "expected standalone test func")
}

func TestRenderer_FixtureWithBeforeAfterEach(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type EachFixture struct {}

func (f *EachFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *EachFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *EachFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *EachFixture) AfterEach(ctx context.Context) error  { return nil }

type EachTestSuite struct {
	*EachFixture
}

func (s *EachTestSuite) BeforeAll(t *gotest.T)  {}
func (s *EachTestSuite) AfterAll(t *gotest.T)   {}
func (s *EachTestSuite) BeforeEach(t *gotest.T) {}
func (s *EachTestSuite) AfterEach(t *gotest.T)  {}
func (s *EachTestSuite) TestCase(t *gotest.T)   {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)
	gotest.True(t, len(output) > 0, "expected non-empty output")

	// Should have the suite wrapper with lifecycle methods delegating
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.BeforeAll(it)"), "expected suite BeforeAll delegation")
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.AfterAll(it)"), "expected suite AfterAll delegation")
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.BeforeEach(it)"), "expected suite BeforeEach delegation")
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.AfterEach(it)"), "expected suite AfterEach delegation")

	// Fixture-level BeforeEach/AfterEach should appear in the test case closure with error handling
	gotest.True(t, strings.Contains(output, "ƒ_EachFixture.BeforeEach(it.Context())"), "expected fixture BeforeEach in test case")
	gotest.True(t, strings.Contains(output, `"EachFixture.BeforeEach failed:`), "expected BeforeEach error attribution")
	gotest.True(t, strings.Contains(output, "ƒ_EachFixture.AfterEach(context.Background())"), "expected fixture AfterEach in test case")
	gotest.True(t, strings.Contains(output, `"EachFixture.AfterEach failed:`), "expected AfterEach error attribution")

	// Verify ordering: fixture AfterEach deferred before suite AfterEach (LIFO)
	fixtureAfterIdx := strings.Index(output, "ƒ_EachFixture.AfterEach(context.Background())")
	suiteAfterIdx := strings.Index(output, "defer s.AfterEach(ttt)")
	gotest.True(t, fixtureAfterIdx < suiteAfterIdx, "fixture AfterEach should be deferred before suite AfterEach (LIFO)")

	fixtureBeforeIdx := strings.Index(output, "ƒ_EachFixture.BeforeEach(it.Context())")
	suiteBeforeIdx := strings.Index(output, "s.BeforeEach(ttt)")
	gotest.True(t, fixtureBeforeIdx < suiteBeforeIdx, "fixture BeforeEach should run before suite BeforeEach")
}

func TestRenderer_FixtureWithoutBeforeAfterEach(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MinimalFixture struct {}

func (f *MinimalFixture) BeforeAll(ctx context.Context) error { return nil }

type MinimalTestSuite struct {
	*MinimalFixture
}

func (s *MinimalTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// Fixture without BeforeEach/AfterEach should NOT emit those calls
	gotest.True(t, !strings.Contains(output, "ƒ_MinimalFixture.BeforeEach"), "should NOT have fixture BeforeEach")
	gotest.True(t, !strings.Contains(output, "ƒ_MinimalFixture.AfterEach"), "should NOT have fixture AfterEach")
}

func TestRenderer_NestedFixtureWithBeforeAfterEach(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type InfraFixture struct {}

func (f *InfraFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *InfraFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *InfraFixture) AfterEach(ctx context.Context) error  { return nil }

type APIFixture struct {
	*InfraFixture
}

func (f *APIFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *APIFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *APIFixture) AfterEach(ctx context.Context) error  { return nil }

type HandlerTestSuite struct {
	*APIFixture
}

func (s *HandlerTestSuite) BeforeEach(t *gotest.T) {}
func (s *HandlerTestSuite) AfterEach(t *gotest.T)  {}
func (s *HandlerTestSuite) TestGet(t *gotest.T)    {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// Nested fixture: parent (fixture) and child hooks should both appear with error handling
	gotest.True(t, strings.Contains(output, "ƒ_InfraFixture.AfterEach(context.Background())"), "expected parent fixture AfterEach")
	gotest.True(t, strings.Contains(output, `"InfraFixture.AfterEach failed:`), "expected parent AfterEach attribution")
	gotest.True(t, strings.Contains(output, "ƒ_InfraFixture.BeforeEach(it.Context())"), "expected parent fixture BeforeEach")
	gotest.True(t, strings.Contains(output, `"InfraFixture.BeforeEach failed:`), "expected parent BeforeEach attribution")
	gotest.True(t, strings.Contains(output, "ƒ_APIFixture.AfterEach(context.Background())"), "expected child fixture AfterEach")
	gotest.True(t, strings.Contains(output, `"APIFixture.AfterEach failed:`), "expected child AfterEach attribution")
	gotest.True(t, strings.Contains(output, "ƒ_APIFixture.BeforeEach(it.Context())"), "expected child fixture BeforeEach")
	gotest.True(t, strings.Contains(output, `"APIFixture.BeforeEach failed:`), "expected child BeforeEach attribution")

	// Verify ordering: parent deferred first (runs last), then child, then suite
	parentAfterIdx := strings.Index(output, "ƒ_InfraFixture.AfterEach(context.Background())")
	childAfterIdx := strings.Index(output, "ƒ_APIFixture.AfterEach(context.Background())")
	suiteAfterIdx := strings.Index(output, "defer s.AfterEach(ttt)")
	gotest.True(t, parentAfterIdx < childAfterIdx, "parent AfterEach deferred before child (LIFO)")
	gotest.True(t, childAfterIdx < suiteAfterIdx, "child AfterEach deferred before suite (LIFO)")

	parentBeforeIdx := strings.Index(output, "ƒ_InfraFixture.BeforeEach(it.Context())")
	childBeforeIdx := strings.Index(output, "ƒ_APIFixture.BeforeEach(it.Context())")
	suiteBeforeIdx := strings.Index(output, "s.BeforeEach(ttt)")
	gotest.True(t, parentBeforeIdx < childBeforeIdx, "parent BeforeEach before child")
	gotest.True(t, childBeforeIdx < suiteBeforeIdx, "child BeforeEach before suite")
}

func TestBuildFixtureViewModels_RootFixtureOnly(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MyFixture struct {}

func (f *MyFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *MyFixture) AfterAll(ctx context.Context) error  { return nil }

type MyTestSuite struct {
	*MyFixture
}

func (s *MyTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	vms := buildFixtureViewModelsFromResolved(resolved.RootFixtures)
	gotest.Equal(t, 1, len(vms))
	gotest.Equal(t, "MyFixture", vms[0].Identifier)
	gotest.True(t, vms[0].BeforeAll, "expected BeforeAll")
	gotest.True(t, vms[0].AfterAll, "expected AfterAll")
	gotest.Equal(t, 1, len(vms[0].ChildSuites))
	gotest.Equal(t, "MyTestSuite", vms[0].ChildSuites[0].Identifier())
	gotest.Equal(t, 0, len(vms[0].ChildFixtures))
}

// --- *testing.T support tests ---

func TestRenderer_StdlibT_StandaloneSuite(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "testing"

type PlainTestSuite struct{}

func (s *PlainTestSuite) BeforeEach(t *testing.T) {}
func (s *PlainTestSuite) AfterEach(t *testing.T)  {}
func (s *PlainTestSuite) TestFoo(t *testing.T)    {}
func (s *PlainTestSuite) TestBar(t *testing.T)    {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// Wrapper lifecycle methods should unwrap via .T()
	gotest.True(t, strings.Contains(output, "ts.PlainTestSuite.BeforeEach(it.T())"), "expected BeforeEach unwrap to .T()")
	gotest.True(t, strings.Contains(output, "ts.PlainTestSuite.AfterEach(it.T())"), "expected AfterEach unwrap to .T()")

	// Test cases should use adapter lambda
	gotest.True(t, strings.Contains(output, `func(t *gotest.T) { s.TestFoo(t.T()) }`), "expected TestFoo adapter")
	gotest.True(t, strings.Contains(output, `func(t *gotest.T) { s.TestBar(t.T()) }`), "expected TestBar adapter")
}

func TestRenderer_StdlibT_MixedSuite(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MixedTestSuite struct{}

func (s *MixedTestSuite) BeforeEach(t *testing.T) {}
func (s *MixedTestSuite) TestStdlib(t *testing.T)  {}
func (s *MixedTestSuite) TestGotest(t *gotest.T)   {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// BeforeEach uses *testing.T → unwrap
	gotest.True(t, strings.Contains(output, "ts.MixedTestSuite.BeforeEach(it.T())"), "expected BeforeEach unwrap")

	// TestStdlib uses *testing.T → adapter
	gotest.True(t, strings.Contains(output, `func(t *gotest.T) { s.TestStdlib(t.T()) }`), "expected TestStdlib adapter")

	// TestGotest uses *gotest.T → direct
	gotest.True(t, strings.Contains(output, `ƒƒ_GOTEST_exec(s.TestGotest, ttt)`), "expected TestGotest direct reference")
	gotest.True(t, !strings.Contains(output, `s.TestGotest(t.T())`), "TestGotest should NOT have adapter")
}

func TestRenderer_StdlibT_FixtureBoundSuite(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"
	"testing"
)

type DBFixture struct{}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type StdlibTestSuite struct {
	*DBFixture
}

func (s *StdlibTestSuite) BeforeAll(t *testing.T)  {}
func (s *StdlibTestSuite) AfterEach(t *testing.T)  {}
func (s *StdlibTestSuite) TestQuery(t *testing.T)  {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// Wrapper lifecycle should unwrap
	gotest.True(t, strings.Contains(output, "ts.StdlibTestSuite.BeforeAll(it.T())"), "expected BeforeAll unwrap")
	gotest.True(t, strings.Contains(output, "ts.StdlibTestSuite.AfterEach(it.T())"), "expected AfterEach unwrap")

	// Test case should use adapter
	gotest.True(t, strings.Contains(output, `func(t *gotest.T) { s.TestQuery(t.T()) }`), "expected TestQuery adapter")
}

func TestRenderer_SharedFixtureEmbedding(t *testing.T) {
	t.Parallel()
	src := "package testpkg\n\n" +
		"import (\n" +
		"\t\"context\"\n\n" +
		"\t\"github.com/mvrahden/go-test/pkg/gotest\"\n" +
		")\n\n" +
		"type PostgresSharedFixture struct {\n" +
		"\tDSN string\n" +
		"}\n\n" +
		"func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error { return nil }\n\n" +
		"type E2EFixture struct {\n" +
		"\t*PostgresSharedFixture\n" +
		"\tPool string\n" +
		"}\n\n" +
		"func (f *E2EFixture) BeforeAll(ctx context.Context) error { return nil }\n" +
		"func (f *E2EFixture) AfterAll(ctx context.Context) error  { return nil }\n\n" +
		"type QueryTestSuite struct {\n" +
		"\t*E2EFixture\n" +
		"}\n\n" +
		"func (s *QueryTestSuite) TestInsert(t *gotest.T) {}\n"

	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)
	gotest.True(t, len(output) > 0, "expected non-empty output")

	// Shared fixture deserialized from JSON state
	gotest.Contains(t, output, "sf0 := &PostgresSharedFixture{}")
	gotest.Contains(t, output, `os.Getenv("GOTEST_SHARED_STATE_FILE")`)
	gotest.Contains(t, output, `"FAIL: GOTEST_SHARED_STATE_FILE not set`)
	gotest.Contains(t, output, `json.Unmarshal`)

	// Shared fixture should be assigned to the package fixture
	gotest.Contains(t, output, "ƒ_E2EFixture.PostgresSharedFixture = sf0")

	// Package fixture lifecycle should still work
	gotest.Contains(t, output, "func ƒƒ_GOTEST_main(m *testing.M)")
	gotest.Contains(t, output, "ƒ_E2EFixture = &E2EFixture{}")
	gotest.Contains(t, output, "ƒ_E2EFixture.BeforeAll(ctx)")
	gotest.Contains(t, output, "ƒ_E2EFixture.AfterAll(ctx)")

	// Suite should be a top-level function
	gotest.Contains(t, output, "func TestQueryTestSuite(t *testing.T)")
	gotest.Contains(t, output, "E2EFixture: ƒ_E2EFixture")

	// JSON state deserialization should appear before ƒ_E2EFixture.BeforeAll
	sfIdx := strings.Index(output, "json.Unmarshal(ƒb, sf0)")
	beforeAllIdx := strings.Index(output, "ƒ_E2EFixture.BeforeAll(ctx)")
	gotest.True(t, sfIdx < beforeAllIdx, "shared fixture JSON deserialization must precede fixture.BeforeAll")
}

func TestRenderer_SharedFixtureEmptyStruct(t *testing.T) {
	t.Parallel()
	src := "package testpkg\n\n" +
		"import (\n" +
		"\t\"context\"\n\n" +
		"\t\"github.com/mvrahden/go-test/pkg/gotest\"\n" +
		")\n\n" +
		"type SetupSharedFixture struct{}\n\n" +
		"func (f *SetupSharedFixture) BeforeAll(ctx context.Context) error { return nil }\n\n" +
		"type AppFixture struct {\n" +
		"\t*SetupSharedFixture\n" +
		"}\n\n" +
		"func (f *AppFixture) BeforeAll(ctx context.Context) error { return nil }\n\n" +
		"type AppTestSuite struct {\n" +
		"\t*AppFixture\n" +
		"}\n\n" +
		"func (s *AppTestSuite) TestRun(t *gotest.T) {}\n"

	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	// Shared fixture should be created and assigned via JSON state
	gotest.Contains(t, output, "sf0 := &SetupSharedFixture{}")
	gotest.Contains(t, output, "ƒ_AppFixture.SetupSharedFixture = sf0")
	gotest.Contains(t, output, `os.Getenv("GOTEST_SHARED_STATE_FILE")`)
}

func TestBuildFixtureViewModels_SharedFixtureDetection(t *testing.T) {
	t.Parallel()
	src := "package testpkg\n\n" +
		"import (\n" +
		"\t\"context\"\n\n" +
		"\t\"github.com/mvrahden/go-test/pkg/gotest\"\n" +
		")\n\n" +
		"type PGSharedFixture struct {\n" +
		"\tDSN  string\n" +
		"\tHost string\n" +
		"}\n\n" +
		"func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }\n\n" +
		"type DBFixture struct {\n" +
		"\t*PGSharedFixture\n" +
		"}\n\n" +
		"func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }\n\n" +
		"type DBTestSuite struct {\n" +
		"\t*DBFixture\n" +
		"}\n\n" +
		"func (s *DBTestSuite) TestQuery(t *gotest.T) {}\n"

	pkg := loadTestPkgWithGotest(t, src)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	vms := buildFixtureViewModelsFromResolved(resolved.RootFixtures)
	gotest.Equal(t, 1, len(vms))

	vm := vms[0]
	gotest.Equal(t, "DBFixture", vm.Identifier)
	gotest.Equal(t, 1, len(vm.SharedFixtures))

	sf := vm.SharedFixtures[0]
	gotest.Equal(t, "sf0", sf.LocalVar)
	gotest.Equal(t, "PGSharedFixture", sf.QualifiedType)
	gotest.Equal(t, "PGSharedFixture", sf.FieldName)
	gotest.Equal(t, "", sf.PkgPath, "same-package shared fixture should have empty PkgPath")
	gotest.Equal(t, pkg.PkgPath+".PGSharedFixture", sf.StateKey)
}

func TestRenderer_FixtureWithConfig(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CFGFixture struct{}

func (f *CFGFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *CFGFixture) AfterAll(ctx context.Context) error  { return nil }
func (f *CFGFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}

type CFGTestSuite struct {
	*CFGFixture
}

func (s *CFGTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "gotest.DefaultFixtureConfig()")
	gotest.Contains(t, output, "gotest.OverlayFixtureConfig(&ƒcfg_CFGFixture, ƒ_CFGFixture.FixtureConfig())")
	gotest.Contains(t, output, "ƒattempts := 1 + ƒcfg_CFGFixture.Retries")
	gotest.Contains(t, output, "ƒ_CFGFixture.BeforeAll(ctx)")
	gotest.Contains(t, output, "context.WithTimeout(ƒctx, ƒcfg_CFGFixture.Timeout)")
}

func TestRenderer_FixtureWithoutConfig_UsesDefault(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PlainFixture struct{}

func (f *PlainFixture) BeforeAll(ctx context.Context) error { return nil }

type PlainTestSuite struct {
	*PlainFixture
}

func (s *PlainTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "gotest.DefaultFixtureConfig()")
	gotest.True(t, !strings.Contains(output, "OverlayFixtureConfig"), "should not have overlay call")
}

func TestRenderer_SuiteWithConfig(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type ConfiguredTestSuite struct{}

func (s *ConfiguredTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 10_000_000_000, FailFast: true}
}
func (s *ConfiguredTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "gotest.DefaultSuiteConfig()")
	gotest.Contains(t, output, "gotest.OverlaySuiteConfig(&ƒcfg, s.ConfiguredTestSuite.SuiteConfig())")
	gotest.Contains(t, output, "gotest.NewTWithDeadline(it, ƒcfg.Timeout)")
	gotest.Contains(t, output, "ƒcfg.FailFast && t.Failed()")
}

func TestRenderer_SuiteWithoutConfig_UsesDefault(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "gotest.DefaultSuiteConfig()")
	gotest.True(t, !strings.Contains(output, "OverlaySuiteConfig"), "should not have overlay call")
}

func TestRenderer_NamedField_SuiteToFixture(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{ Conn string }
func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type QueryTestSuite struct { db *DBFixture }
func (s *QueryTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "db: ƒ_DBFixture", "suite struct literal should use named field")
	gotest.True(t, !strings.Contains(output, "DBFixture: ƒ_DBFixture"), "should NOT use type name as field name")
}

func TestRenderer_NamedField_ChildToParentFixture(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type InfraFixture struct{ Val string }
func (f *InfraFixture) BeforeAll(ctx context.Context) error { return nil }

type APIFixture struct { infra *InfraFixture }
func (f *APIFixture) BeforeAll(ctx context.Context) error { return nil }

type LightTestSuite struct { *InfraFixture }
func (s *LightTestSuite) TestOne(t *gotest.T) {}

type FullTestSuite struct { *APIFixture }
func (s *FullTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "infra: ƒ_InfraFixture", "child fixture struct literal should use named parent field")
}

func TestRenderer_NamedField_SharedFixtureInFixture(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct{ ConnStr string }
func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type AppFixture struct { pg *PGSharedFixture }
func (f *AppFixture) BeforeAll(ctx context.Context) error { return nil }

type UserTestSuite struct { *AppFixture }
func (s *UserTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "ƒ_AppFixture.pg = sf0", "shared fixture injection should use named field")
	gotest.True(t, !strings.Contains(output, "ƒ_AppFixture.PGSharedFixture"), "should NOT use type name for shared fixture field")
}

func TestRenderer_MixedFieldStyles_SameFixture(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{ Conn string }
func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type EmbeddedTestSuite struct { *DBFixture }
func (s *EmbeddedTestSuite) TestOne(t *gotest.T) {}

type NamedTestSuite struct { db *DBFixture }
func (s *NamedTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.Contains(t, output, "DBFixture: ƒ_DBFixture", "embedded suite should use type name")
	gotest.Contains(t, output, "db: ƒ_DBFixture", "named-field suite should use custom field name")
}

func TestRenderer_VoidBeforeEach_Sequential(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type OrderTestSuite struct{}

func (s *OrderTestSuite) BeforeEach(t *gotest.T) {}
func (s *OrderTestSuite) AfterEach(t *gotest.T)  {}
func (s *OrderTestSuite) TestOne(t *gotest.T)    {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.True(t, !strings.Contains(output, "t.Parallel()"), "suite-level t.Parallel() should not be emitted — isolation is subprocess-level")
	gotest.Contains(t, output, "s.BeforeEach(ttt)")
	gotest.Contains(t, output, "defer s.AfterEach(ttt)")
	gotest.True(t, !strings.Contains(output, "sync.WaitGroup"), "sequential suite should not use WaitGroup")
	gotest.True(t, !strings.Contains(output, "it.Parallel()"), "sequential suite should not call it.Parallel()")
}

func TestRenderer_ReturningBeforeEach_Sequential(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{ val string }

type OrderTestSuite struct{}

func (s *OrderTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *OrderTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *OrderTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
func (s *OrderTestSuite) TestTwo(t *gotest.T, _ *myCtx)   {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.True(t, !strings.Contains(output, "t.Parallel()"), "suite-level t.Parallel() should not be emitted")
	gotest.Contains(t, output, "ctx := s.BeforeEach(ttt)")
	gotest.Contains(t, output, "defer s.AfterEach(ttt, ctx)")
	gotest.Contains(t, output, "s.TestOne(ttt, ctx)")
	gotest.Contains(t, output, "s.TestTwo(ttt, ctx)")
	gotest.Contains(t, output, "func (ts *ƒƒ_GOTEST_OrderTestSuite) BeforeEach(it *gotest.T) *myCtx")
	gotest.Contains(t, output, "func (ts *ƒƒ_GOTEST_OrderTestSuite) AfterEach(it *gotest.T, ctx *myCtx)")
}

func TestRenderer_ReturningBeforeEach_Parallel(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{ val string }

type OrderTestSuite struct{}

func (s *OrderTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}
func (s *OrderTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *OrderTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *OrderTestSuite) TestOne(t *gotest.T, ctx *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	stripped := strings.ReplaceAll(output, "it.Parallel()", "")
	gotest.True(t, !strings.Contains(stripped, "t.Parallel()"), "suite-level t.Parallel() should not be emitted")
	gotest.Contains(t, output, "it.Parallel()")
	gotest.Contains(t, output, "wg.Add(1)")
	gotest.Contains(t, output, "defer wg.Done()")
	gotest.Contains(t, output, "wg.Wait()")
	gotest.Contains(t, output, "ctx := s.BeforeEach(ttt)")
	gotest.Contains(t, output, "defer s.AfterEach(ttt, ctx)")
	gotest.Contains(t, output, "s.TestOne(ttt, ctx)")
}

func TestRenderer_FixtureBound_ReturningBeforeEach(t *testing.T) {
	t.Parallel()
	src := `package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{ Conn string }

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type myCtx struct{ pool string }

type QueryTestSuite struct {
	*DBFixture
}

func (s *QueryTestSuite) BeforeEach(t *gotest.T) *myCtx { return &myCtx{} }
func (s *QueryTestSuite) AfterEach(t *gotest.T, ctx *myCtx) {}
func (s *QueryTestSuite) TestInsert(t *gotest.T, ctx *myCtx) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	output, _ := renderTestPkg(t, pkg)

	gotest.True(t, !strings.Contains(output, "t.Parallel()"), "suite-level t.Parallel() should not be emitted")
	gotest.Contains(t, output, "ctx := s.BeforeEach(ttt)")
	gotest.Contains(t, output, "defer s.AfterEach(ttt, ctx)")
	gotest.Contains(t, output, "s.TestInsert(ttt, ctx)")
	gotest.Contains(t, output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeEach(it *gotest.T) *myCtx")
}
