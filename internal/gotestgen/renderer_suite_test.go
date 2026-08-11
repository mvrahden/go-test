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

func renderTestPkg(t testing.TB, pkg *packages.Package, harvestSeeds bool) (string, gotestgen.SpecOutcome) {
	t.Helper()
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Empty(t, result.Errs, "expected no collection errors, got: %v", result.Errs)

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	var resolved *gotestgen.ResolveResult
	if len(spec.EffectiveTestSuites) > 0 {
		resolved, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
		gotest.NoError(t, err)
	}

	r := gotestgen.ExportRenderer{}
	out, err := r.RenderTestSuiteSpec(pkg, spec, resolved, harvestSeeds)
	gotest.NoError(t, err)
	return string(out), spec
}

// loadFuzzHarvestTestPkg loads testdata/fuzzharvest directly with Tests:
// true, so its production file (prod.go) and its _test.go file stay
// distinguishable by filename — the split gotestast.HarvestSeeds' _test.go
// filter depends on. The shared gotestgen.ExportMustTestPkg harness (used by
// every other fixture in this file) batch-loads testdata/sources/*/test.go
// WITHOUT Tests: true, which collapses that distinction, so it can't be used
// for this fixture.
func loadFuzzHarvestTestPkg(t testing.TB) *packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: true,
		Dir:   ".",
	}
	pkgs, err := packages.Load(cfg, "./testdata/fuzzharvest")
	gotest.NoError(t, err)
	for _, p := range pkgs {
		gotest.Empty(t, p.Errors, "package load errors for %s: %v", p.ID, p.Errors)
	}
	for _, p := range pkgs {
		if strings.HasSuffix(p.ID, ".test]") && !strings.HasSuffix(p.Name, "_test") {
			return p
		}
	}
	t.Fatal("expected to find the ptest package variant for testdata/fuzzharvest")
	return nil
}

// --- Fixture rendering tests ---

func (s *RendererTestSuite) TestFixtureRendering(t *gotest.T) {
	t.When("fixture with child suite", func(w *gotest.T) {
		w.It("renders structural elements correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithChildSuite")
			output, _ := renderTestPkg(it.T(), pkg, true)
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
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "ƒ_SimpleFixture.AfterAll", "should NOT have AfterAll call")
		})
	})

	t.When("mixed fixture-bound and standalone", func(w *gotest.T) {
		w.It("renders both fixture-bound and standalone suites", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_MixedFixtureBoundAndStandalone")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "func TestMain(m *testing.M)", "should NOT have TestMain")
			gotest.NotContains(it, output, "RunFixtureMain", "should NOT have RunFixtureMain")
		})
	})

	t.When("fixture with BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("renders lifecycle methods with proper ordering", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("fixture without BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("omits fixture BeforeEach/AfterEach calls", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "ƒ_MinimalFixture.BeforeEach", "should NOT have fixture BeforeEach")
			gotest.NotContains(it, output, "ƒ_MinimalFixture.AfterEach", "should NOT have fixture AfterEach")
		})
	})

	t.When("nested fixture with BeforeEach/AfterEach", func(w *gotest.T) {
		w.It("renders parent and child hooks with proper ordering", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NestedFixtureWithBeforeAfterEach")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- stdlib T support tests ---

func (s *RendererTestSuite) TestAsyncTestCases(t *gotest.T) {
	t.When("a suite has an async test case", func(w *gotest.T) {
		w.It("renders a done channel and deadline wait", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_AsyncTestCases")
			output, _ := renderTestPkg(it.T(), pkg, false)
			gotest.Contains(it, output, "ƒdone := make(chan struct{}, 1)")
			gotest.Contains(it, output, "case <-ƒdone:")
			gotest.Contains(it, output, "done() was not called")
			gotest.NotContains(it, output, "ƒƒ_GOTEST_exec(s.TestPingAsync", "async cases bypass the exec trampoline")
			gotest.Contains(it, output, "ƒƒ_GOTEST_exec(s.TestPlain", "sync cases keep the trampoline")
		})
	})
}

func (s *RendererTestSuite) TestParallelAllExcluded(t *gotest.T) {
	t.It("compiles — no unused ƒfailed when every case is excluded", func(it *gotest.T) {
		pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ParallelAllExcluded")
		output, _ := renderTestPkg(it.T(), pkg, false)
		gotest.NotContains(it, output, "ƒfailed", "ƒfailed must only be declared when test cases exist")
	})
}

func (s *RendererTestSuite) TestParallelFailFast(t *gotest.T) {
	t.When("method-parallel suite with FailFast", func(w *gotest.T) {
		w.It("emits a shared failure flag that skips not-yet-started subtests", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ParallelFailFast")
			output, _ := renderTestPkg(it.T(), pkg, false)
			gotest.Contains(it, output, "ƒfailed.Load()", "parallel subtests must consult the shared failure flag")
			gotest.Contains(it, output, "ƒfailed.Store(true)", "failing subtests must set the shared failure flag")
			gotest.Contains(it, output, "it.Skip(", "flagged subtests must skip instead of running")
		})
	})
}

func (s *RendererTestSuite) TestStdlibTSupport(t *gotest.T) {
	t.When("standalone suite", func(w *gotest.T) {
		w.It("unwraps via .T() and uses adapter lambdas", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_StandaloneSuite")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("mixed suite", func(w *gotest.T) {
		w.It("unwraps stdlib methods and uses direct reference for gotest methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_MixedSuite")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, `s.TestGotest(t.T())`, "TestGotest should NOT have adapter")
		})
	})

	t.When("fixture-bound suite", func(w *gotest.T) {
		w.It("unwraps lifecycle methods and uses adapter for test cases", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_StdlibT_FixtureBoundSuite")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- Shared fixture tests ---

func (s *RendererTestSuite) TestSharedFixture(t *gotest.T) {
	t.When("embedding", func(w *gotest.T) {
		w.It("renders shared fixture as DAG node", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SharedFixtureEmbedding")
			output, _ := renderTestPkg(it.T(), pkg, true)
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
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("empty struct", func(w *gotest.T) {
		w.It("renders shared fixture as DAG node and struct literal wiring", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SharedFixtureEmptyStruct")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- Fixture config tests ---

func (s *RendererTestSuite) TestFixtureConfig(t *gotest.T) {
	t.When("fixture with config", func(w *gotest.T) {
		w.It("uses the marker's config verbatim in the fixture node", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithConfig")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			// The marker method is called once, so the config that bounds the
			// context and the budget it is judged against cannot drift apart. It is
			// called inside ƒ_fixtureOnce.Do rather than at package-variable
			// initialisation: there a panicking FixtureConfig() would abort the
			// binary before TestMain instead of being reported as a setup failure,
			// and it would read the environment TestMain had not set up yet.
			gotest.Contains(it, output, "var ƒcfg_CFGFixture gotest.FixtureConfig", "the config is declared, not derived, at package scope")
			gotest.Contains(it, output, "ƒcfg_CFGFixture = (&CFGFixture{}).FixtureConfig()", "marker config must be used as-is")
			gotest.Contains(it, output,
				"ƒ_fixtureOnce.Do(func() error {\n\t\tƒcfg_CFGFixture = (&CFGFixture{}).FixtureConfig()",
				"the config must be derived inside the containment frame, before anything reads it")
			gotest.Contains(it, output, "Config: ƒcfg_CFGFixture,", "the node reads the hoisted config")
			gotest.Contains(it, output, "Budget: ƒcfg_CFGFixture.Timeout,", "a declared Timeout is also the enforced budget")
			gotest.NotContains(it, output, "OverlayFixtureConfig", "literal semantics: no overlay")
		})
	})

	t.When("fixture without config", func(w *gotest.T) {
		w.It("falls back to the defaults and declares no budget", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureWithoutConfig_UsesDefault")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			// The defaults still bound the fixture's context, but no Budget field
			// is emitted, so it is never failed against a number it did not write.
			gotest.Contains(it, output, "Config: gotest.DefaultFixtureConfig(),",
				"a fixture with no config falls back to the defaults")
			gotest.NotContains(it, output, "Budget:",
				"a fixture with no config must not be held to a budget")
		})
	})
}

// --- Suite config tests ---

func (s *RendererTestSuite) TestSuiteConfig(t *gotest.T) {
	t.When("suite with config", func(w *gotest.T) {
		w.It("uses the marker's config verbatim and renders the deadline", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SuiteWithConfig")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.Contains(it, output, "ƒcfg := s.ConfiguredTestSuite.SuiteConfig()", "marker config must be used as-is")
			gotest.NotContains(it, output, "OverlaySuiteConfig", "literal semantics: no overlay")
		})
	})

	t.When("suite without config", func(w *gotest.T) {
		w.It("falls back to the defaults and declares no budget", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_SuiteWithoutConfig_UsesDefault")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			// The default 30s still bounds t.Context(); the zero budget is what
			// keeps the suite from being failed against it.
			gotest.Contains(it, output, "ƒcfg := gotest.DefaultSuiteConfig()",
				"a suite with no config falls back to the defaults")
			gotest.Contains(it, output, "ƒbudget := gotest.SuiteConfig{}",
				"a suite with no config must declare no budget")
		})
	})
}

func (s *RendererTestSuite) TestUndeclaredBudgetIsZero(t *gotest.T) {
	t.When("a suite declares no SuiteConfig", func(w *gotest.T) {
		w.It("passes a zero budget to every lifecycle phase", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestLifecycle_UndeclaredBudget")
			source, _ := renderTestPkg(it.T(), pkg, false)

			// A suite with no marker method gets the defaults for its contexts
			// and a zero budget, so nothing holds it to a number it never wrote.
			gotest.Contains(it, source, "ƒcfg := gotest.DefaultSuiteConfig()")
			gotest.Contains(it, source, "ƒbudget := gotest.SuiteConfig{}")
			gotest.Contains(it, source, "gotestruntime.RunSetup(t, ƒcfg.SetupTimeout, ƒbudget.SetupTimeout, s.BeforeAll)")
			gotest.Contains(it, source, "gotestruntime.RunTeardown(t, ƒcfg.SetupTimeout, ƒbudget.SetupTimeout, s.AfterAll)")
			gotest.Contains(it, source, "gotestruntime.RunTest(ttt, ƒbudget.Timeout, func() {")
			gotest.NotContains(it, source, "sync.WaitGroup")
		})
	})
}

// --- Named field tests ---

func (s *RendererTestSuite) TestNamedFields(t *gotest.T) {
	t.When("suite to fixture", func(w *gotest.T) {
		w.It("uses named field in struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_SuiteToFixture")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "DBFixture: ƒ_DBFixture", "should NOT use type name as field name")
		})
	})

	t.When("child to parent fixture", func(w *gotest.T) {
		w.It("uses named parent field in struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_ChildToParentFixture")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})

	t.When("shared fixture in fixture", func(w *gotest.T) {
		w.It("uses named field for shared fixture injection via struct literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_NamedField_SharedFixtureInFixture")
			output, _ := renderTestPkg(it.T(), pkg, true)
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
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)
		})
	})
}

// --- BeforeEach rendering tests ---

func (s *RendererTestSuite) TestBeforeEachRendering(t *gotest.T) {
	t.When("void BeforeEach sequential", func(w *gotest.T) {
		w.It("renders sequential suite without parallel markers", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_VoidBeforeEach_Sequential")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted — isolation is subprocess-level")
			gotest.NotContains(it, output, "sync.WaitGroup", "sequential suite should not use WaitGroup")
			gotest.NotContains(it, output, "it.Parallel()", "sequential suite should not call it.Parallel()")
		})
	})

	t.When("returning BeforeEach sequential", func(w *gotest.T) {
		w.It("renders context passing to test methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ReturningBeforeEach_Sequential")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			gotest.NotContains(it, output, "t.Parallel()", "suite-level t.Parallel() should not be emitted")
		})
	})

	t.When("returning BeforeEach parallel", func(w *gotest.T) {
		w.It("renders parallel markers without a WaitGroup", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ReturningBeforeEach_Parallel")
			output, _ := renderTestPkg(it.T(), pkg, true)
			gotest.MatchSnapshot(it, output)

			stripped := strings.ReplaceAll(output, "it.Parallel()", "")
			gotest.NotContains(it, stripped, "t.Parallel()", "suite-level t.Parallel() should not be emitted")

			// A suite-scoped barrier in t.Cleanup deadlocks against Go's panic
			// unwind, which runs ancestor cleanups from the panicking goroutine
			// while the test method that would release the barrier is still
			// parked in t.Run. Go already orders cleanup after all subtests.
			gotest.Contains(it, output, "it.Parallel()", "parallel suite should call it.Parallel()")
			gotest.NotContains(it, output, "sync.WaitGroup", "parallel suite must not gate cleanup on a WaitGroup")
			gotest.NotContains(it, output, "wg.Wait()", "parallel suite must not wait on test methods from t.Cleanup")
			gotest.NotContains(it, output, `"sync"`, "parallel suite should not import sync")
		})
	})

	t.When("parallel suite with every test case excluded", func(w *gotest.T) {
		w.It("imports nothing the harness does not use", func(it *gotest.T) {
			// The suite survives filtering with an empty TestCases slice, so the
			// template emits no ƒfailed atomic.Bool. format.Source does not
			// type-check and would let a stray sync/atomic import through; it is
			// `go test` that then refuses the whole generated package with
			// "imported and not used". So the assertion is on the rendered import
			// block, not on the render succeeding.
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_ParallelSuite_AllCasesExcluded")
			output, _ := renderTestPkg(it.T(), pkg, false)

			gotest.NotContains(it, output, `"sync/atomic"`,
				"an unused sync/atomic import makes go test refuse the generated package")
			gotest.NotContains(it, output, "atomic.Bool",
				"a suite with no test cases has no failure flag to share")
			gotest.Contains(it, output, "func TestAllExcludedTestSuite(t *testing.T)",
				"the suite itself is still rendered")
		})
	})

	t.When("fixture-bound returning BeforeEach", func(w *gotest.T) {
		w.It("renders context passing with fixture binding", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureBound_ReturningBeforeEach")
			output, _ := renderTestPkg(it.T(), pkg, true)
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
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			fixtures := resolved.AllFixtures
			gotest.Len(it, fixtures, 1)
			gotest.Equal(it, "MyFixture", fixtures[0].Identifier)
			gotest.True(it, fixtures[0].BeforeAll, "expected BeforeAll")
			gotest.True(it, fixtures[0].AfterAll, "expected AfterAll")
			gotest.Len(it, fixtures[0].ChildSuites, 1)
			gotest.Equal(it, "MyTestSuite", fixtures[0].ChildSuites[0].Identifier())
		})
	})

	t.When("shared fixture detection", func(w *gotest.T) {
		w.It("detects shared fixture fields and state key", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestBuildFixtureViewModels_SharedFixtureDetection")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			fixtures := resolved.AllFixtures
			gotest.Len(it, fixtures, 1)

			rf := fixtures[0]
			gotest.Equal(it, "DBFixture", rf.Identifier)
			gotest.Len(it, rf.SharedFixtures, 1)

			sf := rf.SharedFixtures[0]
			gotest.Equal(it, "sf0", sf.LocalVar)
			gotest.Equal(it, "PGSharedFixture", sf.QualifiedType)
			gotest.Equal(it, "PGSharedFixture", sf.FieldName)
			gotest.Equal(it, "PGSharedFixture", sf.Identifier)
			gotest.Empty(it, sf.PkgPath, "same-package shared fixture should have empty PkgPath")
			gotest.Equal(it, pkg.PkgPath+".PGSharedFixture", sf.StateKey)
		})
	})
}

// --- Determinism ---

func (s *RendererTestSuite) TestDeterministicOutput(t *gotest.T) {
	t.When("rendering the same package repeatedly", func(w *gotest.T) {
		for sub, tC := range gotest.Each(w, []struct {
			Desc    string
			pkgName string
		}{
			{"parallel suite", "TestRenderer_ReturningBeforeEach_Parallel"},
			{"sequential suite", "TestRenderer_VoidBeforeEach_Sequential"},
			{"fixture-bound suite", "TestRenderer_FixtureWithChildSuite"},
		}) {
			pkg := gotestgen.ExportMustTestPkg(sub.T(), tC.pkgName)
			first, _ := renderTestPkg(sub.T(), pkg, false)
			for range 3 {
				again, _ := renderTestPkg(sub.T(), pkg, false)
				gotest.Equal(sub, first, again, "rendering must be byte-identical across runs")
			}
		}
	})
}

// --- Benchmark wrapper rendering tests ---

func (s *RendererTestSuite) TestRenderer_BenchmarkWrapper(t *gotest.T) {
	t.It("emits Benchmark<Suite> with lifecycle fencing", func(it *gotest.T) {
		pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BenchmarkMethod")
		out, _ := renderTestPkg(it.T(), pkg, true)

		gotest.Contains(it, out, "func BenchmarkBenchTestSuite(b *testing.B)")
		gotest.Contains(it, out, `b.Run("BenchmarkParse"`)
		gotest.Contains(it, out, "b.StopTimer()")
		gotest.Contains(it, out, "s.BeforeEach(ƒeachT)")
		gotest.NotContains(it, out, "X_BenchmarkOld")
	})

	t.It("never emits NewTWithDeadline — benchmarks are bounded by -benchtime, not deadlines", func(it *gotest.T) {
		pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BenchmarkMethod")
		out, _ := renderTestPkg(it.T(), pkg, true)

		benchFn := out[strings.Index(out, "func BenchmarkBenchTestSuite"):]
		gotest.NotContains(it, benchFn, "NewTWithDeadline", "bench wrapper must not apply a suite-config deadline")
	})

	t.When("benchmark method takes *testing.B directly", func(w *gotest.T) {
		w.It("dispatches with the raw *testing.B instead of gotest.NewB(b)", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_Benchmark_StdlibB")
			out, _ := renderTestPkg(it.T(), pkg, true)

			gotest.Contains(it, out, "func BenchmarkStdlibBenchTestSuite(b *testing.B)")
			gotest.Contains(it, out, "s.BenchmarkRaw(b)")
			gotest.NotContains(it, out, "s.BenchmarkRaw(gotest.NewB(b))")
		})
	})

	t.When("benchmark suite is bound to a package fixture", func(w *gotest.T) {
		w.It("calls ƒ_setupFixtures and constructs the suite with fixture fields populated", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureBoundBenchmark")
			out, _ := renderTestPkg(it.T(), pkg, true)

			benchFn := out[strings.Index(out, "func BenchmarkParserTestSuite"):]
			gotest.Contains(it, benchFn, "ƒ_setupFixtures(b)")
			gotest.Contains(it, benchFn, "ParserTestSuite: ParserTestSuite{")
			gotest.Contains(it, benchFn, "PoolFixture: ƒ_PoolFixture")
		})
	})
}

// --- Fuzz wrapper rendering tests ---

func (s *RendererTestSuite) TestRenderer_FuzzWrapper(t *gotest.T) {
	t.It("emits one Fuzz<Suite>_<Method> function per fuzz method, with per-execution lifecycle hooks", func(it *gotest.T) {
		pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FuzzMethod")
		out, _ := renderTestPkg(it.T(), pkg, true)

		gotest.Contains(it, out, "func FuzzFuzzTestSuite_FuzzParse(f *testing.F)")
		gotest.Contains(it, out, "s.FuzzParse(gotest.NewF(f, s.BeforeEach, s.AfterEach))")
		gotest.NotContains(it, out, "X_FuzzOld")
	})

	t.It("wires the suite lifecycle around each generated fuzz function", func(it *gotest.T) {
		pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FuzzMethod")
		out, _ := renderTestPkg(it.T(), pkg, true)

		fuzzFn := out[strings.Index(out, "func FuzzFuzzTestSuite_FuzzParse"):]
		gotest.Contains(it, fuzzFn, "ƒlifecycleT := gotest.NewTFromTB(f)")
		gotest.Contains(it, fuzzFn, "f.Cleanup(func() { s.AfterAll(gotest.NewTFromTB(f)) })")
		gotest.Contains(it, fuzzFn, "s.BeforeAll(ƒlifecycleT)")
	})

	t.When("fuzz suite is bound to a package fixture", func(w *gotest.T) {
		w.It("calls ƒ_setupFixtures and constructs the suite with fixture fields populated", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestRenderer_FixtureBoundFuzz")
			out, _ := renderTestPkg(it.T(), pkg, true)

			fuzzFn := out[strings.Index(out, "func FuzzParserFuzzTestSuite_FuzzParse"):]
			gotest.Contains(it, fuzzFn, "ƒ_setupFixtures(f)")
			gotest.Contains(it, fuzzFn, "ParserFuzzTestSuite: ParserFuzzTestSuite{")
			gotest.Contains(it, fuzzFn, "PoolFixture: ƒ_PoolFixture")
		})
	})

	t.When("the fuzz callback's callee is exercised by a table test elsewhere in the package", func(w *gotest.T) {
		w.It("emits harvested f.Add(...) seed lines before the user method call when harvesting is enabled", func(it *gotest.T) {
			pkg := loadFuzzHarvestTestPkg(it.T())
			out, _ := renderTestPkg(it.T(), pkg, true)

			fuzzFn := out[strings.Index(out, "func FuzzHarvestFuzzTestSuite_FuzzTrim"):]
			gotest.Contains(it, fuzzFn, `f.Add("hello")`)
			gotest.Contains(it, fuzzFn, `f.Add("  hi  ")`)
			gotest.Less(it, strings.Index(fuzzFn, `f.Add(`), strings.Index(fuzzFn, "s.FuzzTrim("), "f.Add(...) lines must precede the user method call")
		})

		w.It("emits no f.Add(...) seed lines when harvesting is disabled", func(it *gotest.T) {
			pkg := loadFuzzHarvestTestPkg(it.T())
			out, _ := renderTestPkg(it.T(), pkg, false)

			fuzzFn := out[strings.Index(out, "func FuzzHarvestFuzzTestSuite_FuzzTrim"):]
			gotest.NotContains(it, fuzzFn, "f.Add(")
		})
	})

	t.When("a fuzz target takes a struct", func(w *gotest.T) {
		w.It("emits the codec source and attaches it to every NewF in the file", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestFuzzCodec_StructTarget")
			out, _ := renderTestPkg(it.T(), pkg, true)

			gotest.Contains(it, out, `"github.com/mvrahden/go-test/pkg/gotestruntime"`)
			gotest.Contains(it, out, "func ƒ_fuzzdec_v1_Request(ƒb []byte) Request {")
			gotest.Contains(it, out, "gotest.Codec[Request]{Decode: ƒ_fuzzdec_v1_Request, Encode: ƒ_fuzzenc_v1_Request, Literal: ƒ_fuzzlit_v1_Request}")
			gotest.Contains(it, out, "s.FuzzCreate(gotest.NewF(f, s.BeforeEach, s.AfterEach, gotest.Codec[Request]{")
			gotest.Contains(it, out, "s.FuzzNative(gotest.NewF(f, s.BeforeEach, s.AfterEach, gotest.Codec[Request]{")
		})

		w.It("emits the codec source exactly once per file", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestFuzzCodec_StructTarget")
			out, _ := renderTestPkg(it.T(), pkg, true)

			gotest.Equal(it, 1, strings.Count(out, "func ƒ_fuzzdec_v1_Request("))
			gotest.Equal(it, 1, strings.Count(out, "func ƒ_fuzzread_v1_Address("))
		})
	})

	t.When("a fuzz target's struct pulls in a type from another package", func(w *gotest.T) {
		w.It("imports that package in the generated header, alongside gotestruntime", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestFuzzCodec_CrossPackage")
			out, _ := renderTestPkg(it.T(), pkg, true)

			gotest.Contains(it, out, `"github.com/mvrahden/go-test/pkg/gotestruntime"`)
			gotest.Contains(it, out, `"testpkg/TestFuzzCodec_CrossDep"`,
				"without this import the generated file references crossdep.Setting and does not compile")
			gotest.Contains(it, out, "crossdep.Setting")
		})
	})

	t.When("no fuzz target needs a codec", func(w *gotest.T) {
		w.It("leaves the NewF call and the import list exactly as before", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FuzzMethod")
			out, _ := renderTestPkg(it.T(), pkg, true)

			gotest.Contains(it, out, "s.FuzzParse(gotest.NewF(f, s.BeforeEach, s.AfterEach))")
			gotest.NotContains(it, out, "ƒ_fuzzdec_")
			// gotestruntime itself is imported by every generated harness
			// (exec sentinel, lifecycle wiring) — codec-free output must
			// merely stay free of the codec primitives built on it.
			gotest.NotContains(it, out, "gotestruntime.NewFuzzReader")
			gotest.NotContains(it, out, "gotestruntime.NewFuzzWriter")
		})
	})
}
