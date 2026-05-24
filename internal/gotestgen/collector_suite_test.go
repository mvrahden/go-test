package gotestgen_test

import (
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type CollectorTestSuite struct{}

func (s *CollectorTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *CollectorTestSuite) TestFixtureCollection(t *gotest.T) {
	t.When("package fixture", func(w *gotest.T) {
		w.It("detects package fixture type and fields", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureCollection_PackageFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Fixtures))
			gotest.Equal(it, gotestast.PackageFixture, result.Fixtures[0].Kind)
			gotest.Equal(it, "DBFixture", result.Fixtures[0].Identifier())
			gotest.True(it, result.Fixtures[0].BeforeAll != nil, "expected BeforeAll to be set")
			gotest.True(it, result.Fixtures[0].AfterAll == nil, "expected AfterAll to be nil")
		})

		w.It("detects all lifecycle methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureCollection_PackageFixtureAllMethods")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Fixtures))

			fix := result.Fixtures[0]
			gotest.True(it, fix.BeforeAll != nil, "expected BeforeAll")
			gotest.True(it, fix.AfterAll != nil, "expected AfterAll")
			gotest.True(it, fix.BeforeEach != nil, "expected BeforeEach")
			gotest.True(it, fix.AfterEach != nil, "expected AfterEach")
		})
	})

	t.When("shared fixture", func(w *gotest.T) {
		w.It("detects shared fixture kind", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureCollection_SharedFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Fixtures))
			gotest.Equal(it, gotestast.SharedFixture, result.Fixtures[0].Kind)
			gotest.True(it, result.Fixtures[0].BeforeAll != nil, "expected BeforeAll to be set")
		})

		w.It("detects shared fixture with AfterAll", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureCollection_SharedFixtureWithAfterAll")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Fixtures))

			fix := result.Fixtures[0]
			gotest.True(it, fix.BeforeAll != nil, "expected BeforeAll")
			gotest.True(it, fix.AfterAll != nil, "expected AfterAll")
		})
	})
}

func (s *CollectorTestSuite) TestFixtureEmbedding(t *gotest.T) {
	t.When("suite embeds fixture", func(w *gotest.T) {
		w.It("detects fixture embedding in test suite", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureEmbeddingInTestSuite")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.Equal(it, 1, len(result.Fixtures))
			gotest.Equal(it, "DBFixture", result.Fixtures[0].Identifier())
		})
	})

	t.When("suite does not embed fixture", func(w *gotest.T) {
		w.It("reports no fixture", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_NoFixtureEmbedding")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.True(it, result.Suites[0].Fixture() == nil, "expected no fixture")
		})
	})

	t.When("fixture embeds fixture", func(w *gotest.T) {
		w.It("collects both fixtures", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureToFixtureEmbedding")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 2, len(result.Fixtures))
		})
	})
}

func (s *CollectorTestSuite) TestSharedFixture(t *gotest.T) {
	t.When("BeforeEach is declared", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_BeforeEachDisallowed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.True(it, len(result.Errs) > 0, "expected error for BeforeEach on shared fixture")
			gotest.Contains(it, result.Errs[0].Err.Error(), "must not have BeforeEach")
		})
	})

	t.When("AfterEach is declared", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_AfterEachDisallowed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.True(it, len(result.Errs) > 0, "expected error for AfterEach on shared fixture")
			gotest.Contains(it, result.Errs[0].Err.Error(), "must not have AfterEach")
		})
	})

	t.When("BeforeAll has wrong signature", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_WrongBeforeAllSignature")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.True(it, len(result.Errs) > 0, "expected error for wrong BeforeAll signature on shared fixture")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported signature")
		})
	})

	t.When("not treated as parent", func(w *gotest.T) {
		w.It("collects both shared and package fixtures", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixtureNotTreatedAsParent")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
			gotest.Equal(it, 1, len(result.Suites))
			gotest.Equal(it, 2, len(result.Fixtures))

			names := map[string]bool{}
			for _, f := range result.Fixtures {
				names[f.Identifier()] = true
			}
			gotest.True(it, names["E2EFixture"], "expected E2EFixture")
			gotest.True(it, names["PGSharedFixture"], "expected PGSharedFixture")
		})
	})

	t.When("Hydrate without Dehydrate", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_HydrateWithoutDehydrate")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: Hydrate without Dehydrate")
			gotest.Contains(it, result.Errs[0].Err.Error(), "has Hydrate but no Dehydrate")
		})
	})

	t.When("Dehydrate without Hydrate", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_DehydrateWithoutHydrate")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: Dehydrate without Hydrate")
			gotest.Contains(it, result.Errs[0].Err.Error(), "has Dehydrate but no Hydrate")
		})
	})
}

func (s *CollectorTestSuite) TestFixtureConfig(t *gotest.T) {
	t.When("fixture has Config method", func(w *gotest.T) {
		w.It("detects config on package fixture", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Fixtures))
			gotest.True(it, result.Fixtures[0].Config != nil, "expected Config to be set")
		})

		w.It("detects config on shared fixture", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixtureConfig_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
			gotest.Equal(it, 1, len(result.Fixtures))
			gotest.True(it, result.Fixtures[0].Config != nil, "expected Config to be set via SharedFixtureConfig")
		})
	})

	t.When("fixture has no Config method", func(w *gotest.T) {
		w.It("reports Config as nil", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_AbsentIsNil")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Fixtures))
			gotest.True(it, result.Fixtures[0].Config == nil, "expected Config to be nil")
		})
	})

	t.When("fixture Config has invalid signature with params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_InvalidSignature_WithParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for invalid FixtureConfig signature")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported signature")
		})
	})

	t.When("fixture Config has wrong return type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_InvalidSignature_WrongReturnType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong FixtureConfig return type")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported return type")
		})
	})
}

func (s *CollectorTestSuite) TestSuiteConfig(t *gotest.T) {
	t.When("suite has Config method", func(w *gotest.T) {
		w.It("detects HasConfig", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.True(it, result.Suites[0].HasConfig(), "expected HasConfig() to be true")
		})
	})

	t.When("suite has no Config method", func(w *gotest.T) {
		w.It("reports HasConfig as false", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_AbsentIsFalse")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.True(it, !result.Suites[0].HasConfig(), "expected HasConfig() to be false")
		})
	})

	t.When("suite Config has invalid signature with params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_InvalidSignature_WithParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for invalid SuiteConfig signature")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported signature")
		})
	})

	t.When("suite Config has wrong return type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_InvalidSignature_WrongReturnType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong SuiteConfig return type")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported return type")
		})
	})

	t.When("parallel is parsed", func(w *gotest.T) {
		w.It("detects IsMethodParallel", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ParallelParsed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
			gotest.True(it, result.Suites[0].IsMethodParallel(), "expected IsMethodParallel to be true")
		})
	})

	t.When("non-literal body", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_NonLiteralBody_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for non-literal SuiteConfig body")
		})
	})
}

func (s *CollectorTestSuite) TestSuiteGuard(t *gotest.T) {
	t.When("suite has Guard method", func(w *gotest.T) {
		w.It("detects HasGuard", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.True(it, result.Suites[0].HasGuard(), "expected HasGuard() to be true")
		})
	})

	t.When("suite has no Guard method", func(w *gotest.T) {
		w.It("reports HasGuard as false", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_AbsentIsFalse")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.True(it, !result.Suites[0].HasGuard(), "expected HasGuard() to be false")
		})
	})

	t.When("Guard has invalid signature with params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_InvalidSignature_WithParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for invalid SuiteGuard signature")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported signature")
		})
	})

	t.When("Guard has wrong return type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_InvalidSignature_WrongReturnType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong SuiteGuard return type")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported return type")
		})
	})
}

func (s *CollectorTestSuite) TestBeforeEach(t *gotest.T) {
	t.When("returning form", func(w *gotest.T) {
		w.It("detects HasReturn on BeforeEach", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BeforeEach_ReturningForm")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
			gotest.Equal(it, 1, len(result.Suites))

			be := result.Suites[0].BeforeEach()
			gotest.True(it, be != nil, "expected BeforeEach")
			gotest.True(it, be.HasReturn(), "expected BeforeEach to have return type")
		})
	})

	t.When("too many returns", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BeforeEach_TooManyReturns")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for 2 return values")
			gotest.Contains(it, result.Errs[0].Err.Error(), "expected 0 or 1 return values")
		})
	})
}

func (s *CollectorTestSuite) TestAfterEach(t *gotest.T) {
	t.When("with context param", func(w *gotest.T) {
		w.It("detects HasContextParam on AfterEach", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_AfterEach_WithContextParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)

			ae := result.Suites[0].AfterEach()
			gotest.True(it, ae != nil, "expected AfterEach")
			gotest.True(it, ae.HasContextParam(), "expected AfterEach to have context param")
		})
	})

	t.When("too many params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_AfterEach_TooManyParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for 3 params")
		})
	})
}

func (s *CollectorTestSuite) TestTestMethod(t *gotest.T) {
	t.When("with context param", func(w *gotest.T) {
		w.It("detects HasContextParam on test methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_TestMethod_WithContextParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
			gotest.Equal(it, 2, len(result.Suites[0].TestCases()))
			gotest.True(it, result.Suites[0].TestCases()[0].HasContextParam(), "expected TestOne to have context param")
			gotest.True(it, result.Suites[0].TestCases()[1].HasContextParam(), "expected TestTwo to have context param")
		})
	})

	t.When("async with context", func(w *gotest.T) {
		w.It("detects HasContextParam on async test method", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_TestMethod_AsyncWithContext")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
			gotest.Equal(it, 1, len(result.Suites[0].TestCases()))
			gotest.True(it, result.Suites[0].TestCases()[0].HasContextParam(), "expected context param")
		})
	})
}

func (s *CollectorTestSuite) TestStdlibT(t *gotest.T) {
	t.When("suite detected", func(w *gotest.T) {
		w.It("detects stdlib T suite and UsesStdlibT", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_SuiteDetected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.Equal(it, "PlainTestSuite", result.Suites[0].Identifier())
			gotest.Equal(it, 1, len(result.Suites[0].TestCases()))
			gotest.True(it, result.Suites[0].TestCases()[0].UsesStdlibT(), "expected UsesStdlibT for *testing.T method")
		})
	})

	t.When("lifecycle hooks", func(w *gotest.T) {
		w.It("detects UsesStdlibT on all lifecycle hooks", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_LifecycleHooks")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))

			suite := result.Suites[0]
			gotest.True(it, suite.BeforeAll() != nil, "expected BeforeAll")
			gotest.True(it, suite.BeforeAll().UsesStdlibT(), "expected BeforeAll UsesStdlibT")
			gotest.True(it, suite.AfterAll() != nil, "expected AfterAll")
			gotest.True(it, suite.AfterAll().UsesStdlibT(), "expected AfterAll UsesStdlibT")
			gotest.True(it, suite.BeforeEach() != nil, "expected BeforeEach")
			gotest.True(it, suite.BeforeEach().UsesStdlibT(), "expected BeforeEach UsesStdlibT")
			gotest.True(it, suite.AfterEach() != nil, "expected AfterEach")
			gotest.True(it, suite.AfterEach().UsesStdlibT(), "expected AfterEach UsesStdlibT")
		})
	})

	t.When("mixed method signatures", func(w *gotest.T) {
		w.It("detects mixed stdlib and gotest T usage", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_MixedMethodSignatures")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites))
			gotest.Equal(it, 2, len(result.Suites[0].TestCases()))

			cases := result.Suites[0].TestCases()
			gotest.Equal(it, "TestStdlib", cases[0].Identifier())
			gotest.True(it, cases[0].UsesStdlibT(), "expected TestStdlib UsesStdlibT")
			gotest.Equal(it, "TestGotest", cases[1].Identifier())
			gotest.True(it, !cases[1].UsesStdlibT(), "expected TestGotest NOT UsesStdlibT")
		})
	})

	t.When("wrong param type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_WrongParamType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.True(it, len(result.Errs) > 0, "expected error for unsupported param type")
			gotest.Contains(it, result.Errs[0].Err.Error(), "must be *gotest.T or *testing.T")
		})
	})
}

func (s *CollectorTestSuite) TestGotestTNotUsesStdlibT(t *gotest.T) {
	t.When("test method uses *gotest.T", func(w *gotest.T) {
		w.It("reports NOT UsesStdlibT", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_GotestT_NotUsesStdlibT")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.Equal(it, 1, len(result.Suites[0].TestCases()))
			gotest.True(it, !result.Suites[0].TestCases()[0].UsesStdlibT(), "expected NOT UsesStdlibT for *gotest.T")
		})
	})
}

func (s *CollectorTestSuite) TestNilPackage(t *gotest.T) {
	t.When("CollectSuiteSpecs receives nil", func(w *gotest.T) {
		w.It("returns empty result", func(it *gotest.T) {
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(nil)
			gotest.Equal(it, 0, len(result.Errs))
			gotest.True(it, result.Suites == nil, "expected nil suites")
			gotest.True(it, result.Fixtures == nil, "expected nil fixtures")
		})
	})
}

func (s *CollectorTestSuite) TestPackageFixtureWrongBeforeAllSignature(t *gotest.T) {
	t.When("BeforeAll has wrong signature", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_PackageFixture_WrongBeforeAllSignature")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.True(it, len(result.Errs) > 0, "expected error for wrong BeforeAll signature on package fixture")
			gotest.Contains(it, result.Errs[0].Err.Error(), "unsupported signature")
		})
	})
}

func (s *CollectorTestSuite) TestValidation(t *gotest.T) {
	t.When("parallel requires returning BeforeEach", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ParallelRequiresReturningBeforeEach")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: parallel requires returning BeforeEach")
			gotest.Contains(it, result.Errs[0].Err.Error(), "Parallel")
		})
	})

	t.When("parallel without BeforeEach", func(w *gotest.T) {
		w.It("is allowed", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ParallelWithoutBeforeEach_Allowed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "parallel with no BeforeEach should be allowed")
		})
	})

	t.When("method missing context param", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_MethodMissingContextParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: TestTwo missing context param")
			gotest.Contains(it, result.Errs[0].Err.Error(), "TestTwo")
		})
	})

	t.When("AfterEach missing context param", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_AfterEachMissingContextParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: AfterEach missing context param")
			gotest.Contains(it, result.Errs[0].Err.Error(), "AfterEach")
		})
	})

	t.When("orphan context AfterEach", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_OrphanContextAfterEach")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: orphan context AfterEach")
		})
	})

	t.When("type mismatch", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_TypeMismatch")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: type mismatch")
			gotest.Contains(it, result.Errs[0].Err.Error(), "does not match")
		})
	})

	t.When("returning BeforeEach fully consistent", func(w *gotest.T) {
		w.It("reports no errors", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ReturningBeforeEach_FullyConsistent_OK")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs), "expected no errors, got: %v", result.Errs)
		})
	})

	t.When("context must be pointer", func(w *gotest.T) {
		w.It("reports an error for non-pointer context", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ContextMustBePointer")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: non-pointer context")
			gotest.Contains(it, result.Errs[0].Err.Error(), "must be a pointer")
		})
	})
}

func (s *CollectorTestSuite) TestApplyTestSuiteSpecs(t *gotest.T) {
	t.When("valid result with fixtures only", func(w *gotest.T) {
		w.It("returns no suites", func(it *gotest.T) {
			c := gotestgen.NewCollector()
			spec, err := c.ApplyTestSuiteSpecs(gotestgen.CollectorResult{
				Fixtures: []*gotestast.FixtureSpec{
					gotestgen.ExportMakeFixtureSpec("Fix1", gotestast.PackageFixture, true),
				},
			})
			gotest.NoError(it, err)
			gotest.True(it, spec.EffectiveTestSuites == nil, "expected no suites")
		})
	})
}
