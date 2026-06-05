package gotestgen_test

import (
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// RendererTestSuite tests Go code generation from suite and fixture specs.
type RendererTestSuite struct{}

func (s *RendererTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func renderTestPkg(t testing.TB, pkg *packages.Package) (string, gotestgen.SpecOutcome) {
	t.Helper()
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs), "expected no collection errors, got: %v", result.Errs)

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	var resolved *gotestgen.ResolveResult
	if len(spec.EffectiveTestSuites) > 0 {
		resolved, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
		gotest.NoError(t, err)
	}

	r := gotestgen.ExportRenderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec, resolved)
	gotest.NoError(t, err)
	return string(out), spec
}

// --- Fixture rendering tests ---

func (s *RendererTestSuite) TestFixtureRendering(t *gotest.T) {
	t.When("fixture with child suite", func(w *gotest.T) {
		w.It("renders structural elements correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithChildSuite")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.True(it, len(output) > 0, "expected non-empty output")

			// Verify the output contains key structural elements
			gotest.Contains(it, output, "func ƒ_setupFixtures(t *testing.T)", "expected ƒ_setupFixtures function")
			gotest.Contains(it, output, "gotestruntime.SetupFixtureDAG(", "expected SetupFixtureDAG call")
			gotest.NotContains(it, output, `"os"`, "should NOT have os import")
			gotest.Contains(it, output, "ƒ_DBFixture = &DBFixture{}", "expected fixture instantiation")
			gotest.Contains(it, output, "ƒ_DBFixture.BeforeAll(ctx)", "expected BeforeAll call")
			gotest.Contains(it, output, "ƒ_DBFixture.AfterAll(ctx)", "expected AfterAll in cleanup")
			gotest.Contains(it, output, "func TestQueryTestSuite(t *testing.T)", "expected top-level TestQueryTestSuite func")
			gotest.Contains(it, output, "ƒƒ_GOTEST_QueryTestSuite", "expected wrapper struct")
			gotest.Contains(it, output, "DBFixture: ƒ_DBFixture", "expected fixture injection")
			gotest.Contains(it, output, `t.Run("TestInsert"`, "expected TestInsert test case")
			gotest.Contains(it, output, `t.Run("TestSelect"`, "expected TestSelect test case")

			// Verify it does NOT contain old-style patterns
			gotest.NotContains(it, output, "func Test_DBFixture(", "should NOT have old-style Test_DBFixture")
			gotest.NotContains(it, output, "go:linkname", "should NOT have linkname directives")

			// Verify wrapper struct and lifecycle methods are at file scope (not nested in functions)
			gotest.Contains(it, output, "type ƒƒ_GOTEST_QueryTestSuite struct", "expected wrapper struct declaration")
			gotest.Contains(it, output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeAll(it *gotest.T)", "expected BeforeAll wrapper")
			gotest.Contains(it, output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeEach(it *gotest.T)", "expected BeforeEach wrapper")
			gotest.Contains(it, output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) AfterEach(it *gotest.T)", "expected AfterEach wrapper")
		})
	})

	t.When("fixture without AfterAll", func(w *gotest.T) {
		w.It("omits AfterAll from cleanup", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutAfterAll")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "gotestruntime.SetupFixtureDAG(", "expected SetupFixtureDAG")
			gotest.NotContains(it, output, "ƒ_SimpleFixture.AfterAll", "should NOT have AfterAll call")
		})
	})

	t.When("mixed fixture-bound and standalone", func(w *gotest.T) {
		w.It("renders both fixture-bound and standalone suites", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_MixedFixtureBoundAndStandalone")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "func ƒ_setupFixtures(t *testing.T)", "expected ƒ_setupFixtures for fixture")
			gotest.Contains(it, output, "gotestruntime.SetupFixtureDAG(", "expected SetupFixtureDAG")
			gotest.Contains(it, output, "func TestBoundTestSuite(t *testing.T)", "expected top-level TestBoundTestSuite func")
			gotest.Contains(it, output, "func TestStandaloneTestSuite(t *testing.T)", "expected standalone test func")
		})
	})

	t.When("fixture with BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("renders lifecycle methods with proper ordering", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.True(it, len(output) > 0, "expected non-empty output")

			// Should have the suite wrapper with lifecycle methods delegating
			gotest.Contains(it, output, "ts.EachTestSuite.BeforeAll(it)", "expected suite BeforeAll delegation")
			gotest.Contains(it, output, "ts.EachTestSuite.AfterAll(it)", "expected suite AfterAll delegation")
			gotest.Contains(it, output, "ts.EachTestSuite.BeforeEach(it)", "expected suite BeforeEach delegation")
			gotest.Contains(it, output, "ts.EachTestSuite.AfterEach(it)", "expected suite AfterEach delegation")

			// Fixture-level BeforeEach/AfterEach should appear in the test case closure with error handling
			gotest.Contains(it, output, "ƒ_EachFixture.BeforeEach(it.Context())", "expected fixture BeforeEach in test case")
			gotest.Contains(it, output, `"EachFixture.BeforeEach failed:`, "expected BeforeEach error attribution")
			gotest.Contains(it, output, "ƒ_EachFixture.AfterEach(context.Background())", "expected fixture AfterEach in test case")
			gotest.Contains(it, output, `"EachFixture.AfterEach failed:`, "expected AfterEach error attribution")

			// Verify ordering: fixture AfterEach deferred before suite AfterEach (LIFO)
			fixtureAfterIdx := strings.Index(output, "ƒ_EachFixture.AfterEach(context.Background())")
			suiteAfterIdx := strings.Index(output, "defer s.AfterEach(ttt)")
			gotest.True(it, fixtureAfterIdx < suiteAfterIdx, "fixture AfterEach should be deferred before suite AfterEach (LIFO)")

			fixtureBeforeIdx := strings.Index(output, "ƒ_EachFixture.BeforeEach(it.Context())")
			suiteBeforeIdx := strings.Index(output, "s.BeforeEach(ttt)")
			gotest.True(it, fixtureBeforeIdx < suiteBeforeIdx, "fixture BeforeEach should run before suite BeforeEach")
		})
	})

	t.When("fixture without BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("omits fixture BeforeEach/AfterEach calls", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.NotContains(it, output, "ƒ_MinimalFixture.BeforeEach", "should NOT have fixture BeforeEach")
			gotest.NotContains(it, output, "ƒ_MinimalFixture.AfterEach", "should NOT have fixture AfterEach")
		})
	})

	t.When("nested fixture with BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("renders parent and child hooks with proper ordering", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NestedFixtureWithBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg)

			// Nested fixture: parent (fixture) and child hooks should both appear with error handling
			gotest.Contains(it, output, "ƒ_InfraFixture.AfterEach(context.Background())", "expected parent fixture AfterEach")
			gotest.Contains(it, output, `"InfraFixture.AfterEach failed:`, "expected parent AfterEach attribution")
			gotest.Contains(it, output, "ƒ_InfraFixture.BeforeEach(it.Context())", "expected parent fixture BeforeEach")
			gotest.Contains(it, output, `"InfraFixture.BeforeEach failed:`, "expected parent BeforeEach attribution")
			gotest.Contains(it, output, "ƒ_APIFixture.AfterEach(context.Background())", "expected child fixture AfterEach")
			gotest.Contains(it, output, `"APIFixture.AfterEach failed:`, "expected child AfterEach attribution")
			gotest.Contains(it, output, "ƒ_APIFixture.BeforeEach(it.Context())", "expected child fixture BeforeEach")
			gotest.Contains(it, output, `"APIFixture.BeforeEach failed:`, "expected child BeforeEach attribution")

			// Verify ordering: parent deferred first (runs last), then child, then suite
			parentAfterIdx := strings.Index(output, "ƒ_InfraFixture.AfterEach(context.Background())")
			childAfterIdx := strings.Index(output, "ƒ_APIFixture.AfterEach(context.Background())")
			suiteAfterIdx := strings.Index(output, "defer s.AfterEach(ttt)")
			gotest.True(it, parentAfterIdx < childAfterIdx, "parent AfterEach deferred before child (LIFO)")
			gotest.True(it, childAfterIdx < suiteAfterIdx, "child AfterEach deferred before suite (LIFO)")

			parentBeforeIdx := strings.Index(output, "ƒ_InfraFixture.BeforeEach(it.Context())")
			childBeforeIdx := strings.Index(output, "ƒ_APIFixture.BeforeEach(it.Context())")
			suiteBeforeIdx := strings.Index(output, "s.BeforeEach(ttt)")
			gotest.True(it, parentBeforeIdx < childBeforeIdx, "parent BeforeEach before child")
			gotest.True(it, childBeforeIdx < suiteBeforeIdx, "child BeforeEach before suite")
		})
	})
}

// --- stdlib T support tests ---

func (s *RendererTestSuite) TestStdlibTSupport(t *gotest.T) {
	t.When("standalone suite", func(w *gotest.T) {
		w.It("unwraps via .T() and uses adapter lambdas", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_StandaloneSuite")
			output, _ := renderTestPkg(it.T(), pkg)

			// Wrapper lifecycle methods should unwrap via .T()
			gotest.Contains(it, output, "ts.PlainTestSuite.BeforeEach(it.T())", "expected BeforeEach unwrap to .T()")
			gotest.Contains(it, output, "ts.PlainTestSuite.AfterEach(it.T())", "expected AfterEach unwrap to .T()")

			// Test cases should use adapter lambda
			gotest.Contains(it, output, `func(t *gotest.T) { s.TestFoo(t.T()) }`, "expected TestFoo adapter")
			gotest.Contains(it, output, `func(t *gotest.T) { s.TestBar(t.T()) }`, "expected TestBar adapter")
		})
	})

	t.When("mixed suite", func(w *gotest.T) {
		w.It("unwraps stdlib methods and uses direct reference for gotest methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_MixedSuite")
			output, _ := renderTestPkg(it.T(), pkg)

			// BeforeEach uses *testing.T -> unwrap
			gotest.Contains(it, output, "ts.MixedTestSuite.BeforeEach(it.T())", "expected BeforeEach unwrap")

			// TestStdlib uses *testing.T -> adapter
			gotest.Contains(it, output, `func(t *gotest.T) { s.TestStdlib(t.T()) }`, "expected TestStdlib adapter")

			// TestGotest uses *gotest.T -> direct
			gotest.Contains(it, output, `ƒƒ_GOTEST_exec(s.TestGotest, ttt)`, "expected TestGotest direct reference")
			gotest.NotContains(it, output, `s.TestGotest(t.T())`, "TestGotest should NOT have adapter")
		})
	})

	t.When("fixture-bound suite", func(w *gotest.T) {
		w.It("unwraps lifecycle methods and uses adapter for test cases", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_FixtureBoundSuite")
			output, _ := renderTestPkg(it.T(), pkg)

			// Wrapper lifecycle should unwrap
			gotest.Contains(it, output, "ts.StdlibTestSuite.BeforeAll(it.T())", "expected BeforeAll unwrap")
			gotest.Contains(it, output, "ts.StdlibTestSuite.AfterEach(it.T())", "expected AfterEach unwrap")

			// Test case should use adapter
			gotest.Contains(it, output, `func(t *gotest.T) { s.TestQuery(t.T()) }`, "expected TestQuery adapter")
		})
	})
}

// --- Shared fixture tests ---

func (s *RendererTestSuite) TestSharedFixture(t *gotest.T) {
	t.When("embedding", func(w *gotest.T) {
		w.It("renders shared fixture as DAG node", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SharedFixtureEmbedding")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.True(it, len(output) > 0, "expected non-empty output")

			// Shared fixture allocated at package level with sf_ prefix
			gotest.Contains(it, output, "var ƒ_sf_PostgresSharedFixture = &PostgresSharedFixture{}")

			// Shared fixture rendered as SharedStateNode DAG node
			gotest.Contains(it, output, "gotestruntime.SharedStateNode")
			gotest.Contains(it, output, "ƒ_sf_PostgresSharedFixture,", "expected Target to reference shared fixture var")

			// Package fixture should wire shared fixture via struct literal
			gotest.Contains(it, output, "PostgresSharedFixture: ƒ_sf_PostgresSharedFixture")

			// Package fixture lifecycle should still work
			gotest.Contains(it, output, "gotestruntime.SetupFixtureDAG(")
			gotest.Contains(it, output, "ƒ_E2EFixture.BeforeAll(ctx)")
			gotest.Contains(it, output, "ƒ_E2EFixture.AfterAll(ctx)")

			// Suite should be a top-level function
			gotest.Contains(it, output, "func TestQueryTestSuite(t *testing.T)")
			gotest.Contains(it, output, "E2EFixture: ƒ_E2EFixture")

			// Package fixture depends on shared fixture
			gotest.Contains(it, output, `"E2EFixture"`)

			// Old patterns should NOT be present
			gotest.NotContains(it, output, "SharedFixtureBinding", "should NOT have old SharedFixtureBinding")
			gotest.NotContains(it, output, "ƒ_sf0_E2EFixture", "should NOT have old sf0 variable naming")
		})
	})

	t.When("cross-package transitive dependency", func(w *gotest.T) {
		w.It("imports the transitive shared fixture package", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_CrossPkgTransitiveSharedFixture")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.True(it, len(output) > 0, "expected non-empty output")

			gotest.Contains(it, output, `"testpkg/extfixtures"`, "expected import for transitive shared fixture package")
			gotest.Contains(it, output, "var ƒ_sf_extfixtures_PostgresSharedFixture = &extfixtures.PostgresSharedFixture{}")
			gotest.Contains(it, output, "var ƒ_sf_SchemaSharedFixture = &SchemaSharedFixture{}")
		})
	})

	t.When("empty struct", func(w *gotest.T) {
		w.It("renders shared fixture as DAG node and struct literal wiring", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SharedFixtureEmptyStruct")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "var ƒ_sf_SetupSharedFixture = &SetupSharedFixture{}")
			gotest.Contains(it, output, "SetupSharedFixture: ƒ_sf_SetupSharedFixture")
			gotest.Contains(it, output, "ƒ_sf_SetupSharedFixture,", "expected Target to reference shared fixture var")
		})
	})
}

// --- Fixture config tests ---

func (s *RendererTestSuite) TestFixtureConfig(t *gotest.T) {
	t.When("fixture with config", func(w *gotest.T) {
		w.It("renders config overlay in fixture node", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithConfig")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "gotest.DefaultFixtureConfig()")
			gotest.Contains(it, output, "gotest.OverlayFixtureConfig(&cfg, (&CFGFixture{}).FixtureConfig())")
			gotest.Contains(it, output, "ƒ_CFGFixture.BeforeAll(ctx)")
			gotest.Contains(it, output, `Name: "CFGFixture"`)
		})
	})

	t.When("fixture without config", func(w *gotest.T) {
		w.It("uses default config without overlay", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutConfig_UsesDefault")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "gotest.DefaultFixtureConfig()")
			gotest.NotContains(it, output, "OverlayFixtureConfig", "should not have overlay call")
		})
	})
}

// --- Suite config tests ---

func (s *RendererTestSuite) TestSuiteConfig(t *gotest.T) {
	t.When("suite with config", func(w *gotest.T) {
		w.It("renders config overlay and deadline", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SuiteWithConfig")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "gotest.DefaultSuiteConfig()")
			gotest.Contains(it, output, "gotest.OverlaySuiteConfig(&ƒcfg, s.ConfiguredTestSuite.SuiteConfig())")
			gotest.Contains(it, output, "gotest.NewTWithDeadline(it, ƒcfg.Timeout)")
			gotest.Contains(it, output, "ƒcfg.FailFast && t.Failed()")
		})
	})

	t.When("suite without config", func(w *gotest.T) {
		w.It("uses default config without overlay", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SuiteWithoutConfig_UsesDefault")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "gotest.DefaultSuiteConfig()")
			gotest.NotContains(it, output, "OverlaySuiteConfig", "should not have overlay call")
		})
	})
}

// --- Named field tests ---

func (s *RendererTestSuite) TestNamedFields(t *gotest.T) {
	t.When("suite to fixture", func(w *gotest.T) {
		w.It("uses named field in struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_SuiteToFixture")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "db: ƒ_DBFixture", "suite struct literal should use named field")
			gotest.NotContains(it, output, "DBFixture: ƒ_DBFixture", "should NOT use type name as field name")
		})
	})

	t.When("child to parent fixture", func(w *gotest.T) {
		w.It("uses named parent field in struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_ChildToParentFixture")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "infra: ƒ_InfraFixture", "child fixture struct literal should use named parent field")
		})
	})

	t.When("shared fixture in fixture", func(w *gotest.T) {
		w.It("uses named field for shared fixture injection via struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_SharedFixtureInFixture")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "pg: ƒ_sf_PGSharedFixture", "shared fixture injection should use named field in struct literal")
			gotest.NotContains(it, output, "ƒ_AppFixture.PGSharedFixture", "should NOT use type name for shared fixture field")
			gotest.NotContains(it, output, "ƒ_sf0_AppFixture", "should NOT have old sf0 variable naming")
		})
	})
}

// --- Mixed field styles test ---

func (s *RendererTestSuite) TestMixedFieldStyles(t *gotest.T) {
	t.When("same fixture with embedded and named fields", func(w *gotest.T) {
		w.It("uses type name for embedded and custom name for named field", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_MixedFieldStyles_SameFixture")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.Contains(it, output, "DBFixture: ƒ_DBFixture", "embedded suite should use type name")
			gotest.Contains(it, output, "db: ƒ_DBFixture", "named-field suite should use custom field name")
		})
	})
}

// --- BeforeEach rendering tests ---

func (s *RendererTestSuite) TestBeforeEachRendering(t *gotest.T) {
	t.When("void BeforeEach sequential", func(w *gotest.T) {
		w.It("renders sequential suite without parallel markers", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_VoidBeforeEach_Sequential")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted — isolation is subprocess-level")
			gotest.Contains(it, output, "s.BeforeEach(ttt)")
			gotest.Contains(it, output, "defer s.AfterEach(ttt)")
			gotest.NotContains(it, output, "sync.WaitGroup", "sequential suite should not use WaitGroup")
			gotest.NotContains(it, output, "it.Parallel()", "sequential suite should not call it.Parallel()")
		})
	})

	t.When("returning BeforeEach sequential", func(w *gotest.T) {
		w.It("renders context passing to test methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ReturningBeforeEach_Sequential")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted")
			gotest.Contains(it, output, "ctx := s.BeforeEach(ttt)")
			gotest.Contains(it, output, "defer s.AfterEach(ttt, ctx)")
			gotest.Contains(it, output, "s.TestOne(ttt, ctx)")
			gotest.Contains(it, output, "s.TestTwo(ttt, ctx)")
			gotest.Contains(it, output, "func (ts *ƒƒ_GOTEST_OrderTestSuite) BeforeEach(it *gotest.T) *myCtx")
			gotest.Contains(it, output, "func (ts *ƒƒ_GOTEST_OrderTestSuite) AfterEach(it *gotest.T, ctx *myCtx)")
		})
	})

	t.When("returning BeforeEach parallel", func(w *gotest.T) {
		w.It("renders parallel markers and WaitGroup", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ReturningBeforeEach_Parallel")
			output, _ := renderTestPkg(it.T(), pkg)

			stripped := strings.ReplaceAll(output, "it.Parallel()", "")
			gotest.NotContains(it, stripped, "t.Parallel()", "suite-level t.Parallel() should not be emitted")
			gotest.Contains(it, output, "it.Parallel()")
			gotest.Contains(it, output, "wg.Add(1)")
			gotest.Contains(it, output, "defer wg.Done()")
			gotest.Contains(it, output, "wg.Wait()")
			gotest.Contains(it, output, "ctx := s.BeforeEach(ttt)")
			gotest.Contains(it, output, "defer s.AfterEach(ttt, ctx)")
			gotest.Contains(it, output, "s.TestOne(ttt, ctx)")
		})
	})

	t.When("fixture-bound returning BeforeEach", func(w *gotest.T) {
		w.It("renders context passing with fixture binding", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureBound_ReturningBeforeEach")
			output, _ := renderTestPkg(it.T(), pkg)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted")
			gotest.Contains(it, output, "ctx := s.BeforeEach(ttt)")
			gotest.Contains(it, output, "defer s.AfterEach(ttt, ctx)")
			gotest.Contains(it, output, "s.TestInsert(ttt, ctx)")
			gotest.Contains(it, output, "func (ts *ƒƒ_GOTEST_QueryTestSuite) BeforeEach(it *gotest.T) *myCtx")
		})
	})
}

// --- Resolved fixture tests ---

func (s *RendererTestSuite) TestResolvedFixtures(t *gotest.T) {
	t.When("root fixture only", func(w *gotest.T) {
		w.It("resolves correct fixture structure", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestBuildFixtureViewModels_RootFixtureOnly")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			fixtures := resolved.AllFixtures
			gotest.Equal(it, 1, len(fixtures))
			gotest.Equal(it, "MyFixture", fixtures[0].Identifier)
			gotest.True(it, fixtures[0].BeforeAll, "expected BeforeAll")
			gotest.True(it, fixtures[0].AfterAll, "expected AfterAll")
			gotest.Equal(it, 1, len(fixtures[0].ChildSuites))
			gotest.Equal(it, "MyTestSuite", fixtures[0].ChildSuites[0].Identifier())
		})
	})

	t.When("shared fixture detection", func(w *gotest.T) {
		w.It("detects shared fixture fields and state key", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestBuildFixtureViewModels_SharedFixtureDetection")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			fixtures := resolved.AllFixtures
			gotest.Equal(it, 1, len(fixtures))

			rf := fixtures[0]
			gotest.Equal(it, "DBFixture", rf.Identifier)
			gotest.Equal(it, 1, len(rf.SharedFixtures))

			sf := rf.SharedFixtures[0]
			gotest.Equal(it, "sf0", sf.LocalVar)
			gotest.Equal(it, "PGSharedFixture", sf.QualifiedType)
			gotest.Equal(it, "PGSharedFixture", sf.FieldName)
			gotest.Equal(it, "PGSharedFixture", sf.Identifier)
			gotest.Equal(it, "", sf.PkgPath, "same-package shared fixture should have empty PkgPath")
			gotest.Equal(it, pkg.PkgPath+".PGSharedFixture", sf.StateKey)
		})
	})
}
