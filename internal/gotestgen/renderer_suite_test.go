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
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "func TestMain(m *testing.M)", "should NOT have TestMain")
			gotest.NotContains(it, output, "RunFixtureMain", "should NOT have RunFixtureMain")
			gotest.NotContains(it, output, "func Test_DBFixture(", "should NOT have old-style Test_DBFixture")
			gotest.NotContains(it, output, "go:linkname", "should NOT have linkname directives")
		})
	})

	t.When("fixture without AfterAll", func(w *gotest.T) {
		w.It("omits AfterAll from cleanup", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutAfterAll")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "ƒ_SimpleFixture.AfterAll", "should NOT have AfterAll call")
		})
	})

	t.When("mixed fixture-bound and standalone", func(w *gotest.T) {
		w.It("renders both fixture-bound and standalone suites", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_MixedFixtureBoundAndStandalone")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "func TestMain(m *testing.M)", "should NOT have TestMain")
			gotest.NotContains(it, output, "RunFixtureMain", "should NOT have RunFixtureMain")
		})
	})

	t.When("fixture with BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("renders lifecycle methods with proper ordering", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("fixture without BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("omits fixture BeforeEach/AfterEach calls", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "ƒ_MinimalFixture.BeforeEach", "should NOT have fixture BeforeEach")
			gotest.NotContains(it, output, "ƒ_MinimalFixture.AfterEach", "should NOT have fixture AfterEach")
		})
	})

	t.When("nested fixture with BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("renders parent and child hooks with proper ordering", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NestedFixtureWithBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- stdlib T support tests ---

func (s *RendererTestSuite) TestStdlibTSupport(t *gotest.T) {
	t.When("standalone suite", func(w *gotest.T) {
		w.It("unwraps via .T() and uses adapter lambdas", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_StandaloneSuite")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("mixed suite", func(w *gotest.T) {
		w.It("unwraps stdlib methods and uses direct reference for gotest methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_MixedSuite")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, `s.TestGotest(t.T())`, "TestGotest should NOT have adapter")
		})
	})

	t.When("fixture-bound suite", func(w *gotest.T) {
		w.It("unwraps lifecycle methods and uses adapter for test cases", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_FixtureBoundSuite")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- Shared fixture tests ---

func (s *RendererTestSuite) TestSharedFixture(t *gotest.T) {
	t.When("embedding", func(w *gotest.T) {
		w.It("renders shared fixture as DAG node", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SharedFixtureEmbedding")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "func TestMain(m *testing.M)", "should NOT have TestMain")
			gotest.NotContains(it, output, "RunFixtureMain", "should NOT have RunFixtureMain")
			gotest.NotContains(it, output, "SharedFixtureBinding", "should NOT have old SharedFixtureBinding")
			gotest.NotContains(it, output, "ƒ_sf0_E2EFixture", "should NOT have old sf0 variable naming")
		})
	})

	t.When("cross-package transitive dependency", func(w *gotest.T) {
		w.It("imports the transitive shared fixture package", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_CrossPkgTransitiveSharedFixture")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("empty struct", func(w *gotest.T) {
		w.It("renders shared fixture as DAG node and struct literal wiring", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SharedFixtureEmptyStruct")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- Fixture config tests ---

func (s *RendererTestSuite) TestFixtureConfig(t *gotest.T) {
	t.When("fixture with config", func(w *gotest.T) {
		w.It("renders config overlay in fixture node", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithConfig")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("fixture without config", func(w *gotest.T) {
		w.It("uses default config without overlay", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutConfig_UsesDefault")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

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
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("suite without config", func(w *gotest.T) {
		w.It("uses default config without overlay", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SuiteWithoutConfig_UsesDefault")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

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
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "DBFixture: ƒ_DBFixture", "should NOT use type name as field name")
		})
	})

	t.When("child to parent fixture", func(w *gotest.T) {
		w.It("uses named parent field in struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_ChildToParentFixture")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("shared fixture in fixture", func(w *gotest.T) {
		w.It("uses named field for shared fixture injection via struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_SharedFixtureInFixture")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

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
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- BeforeEach rendering tests ---

func (s *RendererTestSuite) TestBeforeEachRendering(t *gotest.T) {
	t.When("void BeforeEach sequential", func(w *gotest.T) {
		w.It("renders sequential suite without parallel markers", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_VoidBeforeEach_Sequential")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted — isolation is subprocess-level")
			gotest.NotContains(it, output, "sync.WaitGroup", "sequential suite should not use WaitGroup")
			gotest.NotContains(it, output, "it.Parallel()", "sequential suite should not call it.Parallel()")
		})
	})

	t.When("returning BeforeEach sequential", func(w *gotest.T) {
		w.It("renders context passing to test methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ReturningBeforeEach_Sequential")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted")
		})
	})

	t.When("returning BeforeEach parallel", func(w *gotest.T) {
		w.It("renders parallel markers and WaitGroup", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ReturningBeforeEach_Parallel")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			stripped := strings.ReplaceAll(output, "it.Parallel()", "")
			gotest.NotContains(it, stripped, "t.Parallel()", "suite-level t.Parallel() should not be emitted")
		})
	})

	t.When("fixture-bound returning BeforeEach", func(w *gotest.T) {
		w.It("renders context passing with fixture binding", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureBound_ReturningBeforeEach")
			output, _ := renderTestPkg(it.T(), pkg)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted")
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
