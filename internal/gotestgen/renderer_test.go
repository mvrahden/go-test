package gotestgen

import (
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestRenderer_FixtureWithChildSuite(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type DBFixture struct {
	Conn string
}

func (f *DBFixture) BeforeAll(t *gotest.T)  {}
func (f *DBFixture) AfterAll(t *gotest.T)   {}

type QueryTestSuite struct {
	*DBFixture
}

func (s *QueryTestSuite) BeforeEach(t *gotest.T) {}
func (s *QueryTestSuite) AfterEach(t *gotest.T)  {}
func (s *QueryTestSuite) TestInsert(t *gotest.T) {}
func (s *QueryTestSuite) TestSelect(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)

	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))
	gotest.Equal(t, 1, len(result.Suites))
	gotest.Equal(t, 1, len(result.Fixtures))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	gotest.Equal(t, 1, len(spec.EffectiveTestSuites))
	gotest.Equal(t, 1, len(spec.Fixtures))

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)
	gotest.True(t, len(out) > 0, "expected non-empty output")

	output := string(out)

	// Verify the output contains key structural elements
	gotest.True(t, strings.Contains(output, "func TestMain(m *testing.M)"), "expected TestMain")
	gotest.True(t, strings.Contains(output, "os.Exit(m.Run())"), "expected os.Exit(m.Run())")
	gotest.True(t, strings.Contains(output, "func Test_DBFixture(t *testing.T)"), "expected Test_DBFixture")
	gotest.True(t, strings.Contains(output, `"os"`), "expected os import")
	gotest.True(t, strings.Contains(output, "fixture := &DBFixture{}"), "expected fixture instantiation")
	gotest.True(t, strings.Contains(output, "fixture.BeforeAll(ft)"), "expected BeforeAll call")
	gotest.True(t, strings.Contains(output, "fixture.AfterAll(ft)"), "expected AfterAll in cleanup")
	gotest.True(t, strings.Contains(output, `t.Run("QueryTestSuite"`), "expected t.Run for child suite")
	gotest.True(t, strings.Contains(output, "ƒƒ_GOTEST_QueryTestSuite"), "expected wrapper struct")
	gotest.True(t, strings.Contains(output, "DBFixture: fixture"), "expected fixture injection")
	gotest.True(t, strings.Contains(output, `newTestCase("TestInsert"`), "expected TestInsert test case")
	gotest.True(t, strings.Contains(output, `newTestCase("TestSelect"`), "expected TestSelect test case")

	// Verify it does NOT contain standalone Test function
	gotest.True(t, !strings.Contains(output, "func TestQueryTestSuite("), "should NOT have standalone TestQueryTestSuite")

	// Verify wrapper struct and lifecycle methods are at file scope (not nested in functions)
	gotest.True(t, strings.Contains(output, "type ƒƒ_GOTEST_QueryTestSuite struct"), "expected wrapper struct declaration")
	gotest.True(t, strings.Contains(output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeAll(it *gotest.T)"), "expected BeforeAll wrapper")
	gotest.True(t, strings.Contains(output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeEach(it *gotest.T)"), "expected BeforeEach wrapper")
	gotest.True(t, strings.Contains(output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) AfterEach(it *gotest.T)"), "expected AfterEach wrapper")
}

func TestRenderer_FixtureWithoutAfterAll(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type SimpleFixture struct {}

func (f *SimpleFixture) BeforeAll(t *gotest.T) {}

type BasicTestSuite struct {
	*SimpleFixture
}

func (s *BasicTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)

	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)

	output := string(out)

	// AfterAll should NOT be in the cleanup since the fixture has no AfterAll
	gotest.True(t, strings.Contains(output, "func Test_SimpleFixture(t *testing.T)"), "expected Test_SimpleFixture")
	gotest.True(t, !strings.Contains(output, "fixture.AfterAll"), "should NOT have AfterAll call")
}

func TestRenderer_MixedFixtureBoundAndStandalone(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type AppFixture struct {}

func (f *AppFixture) BeforeAll(t *gotest.T) {}

type BoundTestSuite struct {
	*AppFixture
}

func (s *BoundTestSuite) TestBound(t *gotest.T) {}

type StandaloneTestSuite struct {}

func (s *StandaloneTestSuite) TestFree(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)

	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	gotest.Equal(t, 2, len(spec.EffectiveTestSuites))

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)

	output := string(out)

	// Should have both fixture-bound and standalone
	gotest.True(t, strings.Contains(output, "func TestMain(m *testing.M)"), "expected TestMain for fixture")
	gotest.True(t, strings.Contains(output, "func Test_AppFixture(t *testing.T)"), "expected fixture test")
	gotest.True(t, strings.Contains(output, `t.Run("BoundTestSuite"`), "expected bound suite in t.Run")
	gotest.True(t, strings.Contains(output, "func TestStandaloneTestSuite(t *testing.T)"), "expected standalone test func")
}

func TestRenderer_FixtureWithBeforeAfterEach(t *testing.T) {
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type EachFixture struct {}

func (f *EachFixture) BeforeAll(t *gotest.T)  {}
func (f *EachFixture) AfterAll(t *gotest.T)   {}
func (f *EachFixture) BeforeEach(t *gotest.T) {}
func (f *EachFixture) AfterEach(t *gotest.T)  {}

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

	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	r := renderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec)
	gotest.NoError(t, err)

	output := string(out)
	gotest.True(t, len(output) > 0, "expected non-empty output")

	// Should have the suite wrapper with lifecycle methods delegating
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.BeforeAll(it)"), "expected suite BeforeAll delegation")
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.AfterAll(it)"), "expected suite AfterAll delegation")
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.BeforeEach(it)"), "expected suite BeforeEach delegation")
	gotest.True(t, strings.Contains(output, "ts.EachTestSuite.AfterEach(it)"), "expected suite AfterEach delegation")
}

func TestBuildFixtureViewModels_RootFixtureOnly(t *testing.T) {
	c := collector{}
	src := `package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type MyFixture struct {}

func (f *MyFixture) BeforeAll(t *gotest.T) {}
func (f *MyFixture) AfterAll(t *gotest.T)  {}

type MyTestSuite struct {
	*MyFixture
}

func (s *MyTestSuite) TestOne(t *gotest.T) {}
`
	pkg := loadTestPkgWithGotest(t, src)
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	fixtureBound, standalone := splitSuitesByFixture(spec)
	gotest.Equal(t, 1, len(fixtureBound))
	gotest.Equal(t, 0, len(standalone))

	vms := buildFixtureViewModels(spec.Fixtures, fixtureBound)
	gotest.Equal(t, 1, len(vms))
	gotest.Equal(t, "MyFixture", vms[0].Identifier)
	gotest.True(t, vms[0].BeforeAll, "expected BeforeAll")
	gotest.True(t, vms[0].AfterAll, "expected AfterAll")
	gotest.Equal(t, 1, len(vms[0].ChildSuites))
	gotest.Equal(t, "MyTestSuite", vms[0].ChildSuites[0].Identifier())
	gotest.Equal(t, 0, len(vms[0].ChildFixtures))
}
