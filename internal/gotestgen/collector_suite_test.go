package gotestgen_test

import (
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CollectorTestSuite tests suite and fixture spec collection from Go packages.
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
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Fixtures, 1)
			gotest.Equal(it, gotestast.PackageFixture, result.Fixtures[0].Kind)
			gotest.Equal(it, "DBFixture", result.Fixtures[0].Identifier())
			gotest.NotZero(it, result.Fixtures[0].BeforeAll, "expected BeforeAll to be set")
			gotest.Zero(it, result.Fixtures[0].AfterAll, "expected AfterAll to be nil")
		})

		w.It("detects all lifecycle methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureCollection_PackageFixtureAllMethods")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Fixtures, 1)

			fix := result.Fixtures[0]
			gotest.NotZero(it, fix.BeforeAll, "expected BeforeAll")
			gotest.NotZero(it, fix.AfterAll, "expected AfterAll")
			gotest.NotZero(it, fix.BeforeEach, "expected BeforeEach")
			gotest.NotZero(it, fix.AfterEach, "expected AfterEach")
		})
	})

	t.When("shared fixture", func(w *gotest.T) {
		w.It("detects shared fixture kind", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureCollection_SharedFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Fixtures, 1)
			gotest.Equal(it, gotestast.SharedFixture, result.Fixtures[0].Kind)
			gotest.NotZero(it, result.Fixtures[0].BeforeAll, "expected BeforeAll to be set")
		})

		w.It("detects shared fixture with AfterAll", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureCollection_SharedFixtureWithAfterAll")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Fixtures, 1)

			fix := result.Fixtures[0]
			gotest.NotZero(it, fix.BeforeAll, "expected BeforeAll")
			gotest.NotZero(it, fix.AfterAll, "expected AfterAll")
		})
	})
}

func (s *CollectorTestSuite) TestFixtureEmbedding(t *gotest.T) {
	t.When("suite embeds fixture", func(w *gotest.T) {
		w.It("detects fixture embedding in test suite", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureEmbeddingInTestSuite")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.Len(it, result.Fixtures, 1)
			gotest.Equal(it, "DBFixture", result.Fixtures[0].Identifier())
		})
	})

	t.When("suite does not embed fixture", func(w *gotest.T) {
		w.It("reports no fixture", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_NoFixtureEmbedding")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.Zero(it, result.Suites[0].Fixture(), "expected no fixture")
		})
	})

	t.When("fixture embeds fixture", func(w *gotest.T) {
		w.It("collects both fixtures", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureToFixtureEmbedding")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Fixtures, 2)
		})
	})

	t.When("package fixture has wrong BeforeAll signature", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_PackageFixture_WrongBeforeAllSignature")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong BeforeAll signature on package fixture")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported signature")
		})
	})
}

func (s *CollectorTestSuite) TestSharedFixture(t *gotest.T) {
	t.When("BeforeEach is declared", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_BeforeEachDisallowed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for BeforeEach on shared fixture")
			gotest.ErrorContains(it, result.Errs[0].Err, "must not have BeforeEach")
		})
	})

	t.When("AfterEach is declared", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_AfterEachDisallowed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for AfterEach on shared fixture")
			gotest.ErrorContains(it, result.Errs[0].Err, "must not have AfterEach")
		})
	})

	t.When("BeforeAll has wrong signature", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_WrongBeforeAllSignature")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong BeforeAll signature on shared fixture")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported signature")
		})
	})

	t.When("not treated as parent", func(w *gotest.T) {
		w.It("collects both shared and package fixtures", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixtureNotTreatedAsParent")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.Len(it, result.Fixtures, 2)

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
			gotest.ErrorContains(it, result.Errs[0].Err, "has Hydrate but no Dehydrate")
		})
	})

	t.When("Dehydrate without Hydrate", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixture_DehydrateWithoutHydrate")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: Dehydrate without Hydrate")
			gotest.ErrorContains(it, result.Errs[0].Err, "has Dehydrate but no Hydrate")
		})
	})
}

func (s *CollectorTestSuite) TestFixtureConfig(t *gotest.T) {
	t.When("fixture has Config method", func(w *gotest.T) {
		w.It("detects config on package fixture", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Fixtures, 1)
			gotest.NotZero(it, result.Fixtures[0].Config, "expected Config to be set")
		})

		w.It("detects config on shared fixture", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SharedFixtureConfig_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.Len(it, result.Fixtures, 1)
			gotest.NotZero(it, result.Fixtures[0].Config, "expected Config to be set via SharedFixtureConfig")
		})
	})

	t.When("fixture has no Config method", func(w *gotest.T) {
		w.It("reports Config as nil", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_AbsentIsNil")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Fixtures, 1)
			gotest.Zero(it, result.Fixtures[0].Config, "expected Config to be nil")
		})
	})

	t.When("fixture Config has invalid signature with params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_InvalidSignature_WithParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for invalid FixtureConfig signature")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported signature")
		})
	})

	t.When("fixture Config has wrong return type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_FixtureConfig_InvalidSignature_WrongReturnType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong FixtureConfig return type")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported return type")
		})
	})
}

func (s *CollectorTestSuite) TestSuiteConfig(t *gotest.T) {
	t.When("suite has Config method", func(w *gotest.T) {
		w.It("detects HasConfig", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.True(it, result.Suites[0].HasConfig(), "expected HasConfig() to be true")
		})
	})

	t.When("suite composes its config from a preset", func(w *gotest.T) {
		w.It("accepts the compose form and detects Parallel", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ComposeBody")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.True(it, result.Suites[0].HasConfig(), "expected HasConfig() to be true")
			gotest.True(it, result.Suites[0].IsMethodParallel(), "Parallel set via cfg.Parallel = true must be detected")
		})
	})

	t.When("suite has no Config method", func(w *gotest.T) {
		w.It("reports HasConfig as false", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_AbsentIsFalse")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.False(it, result.Suites[0].HasConfig(), "expected HasConfig() to be false")
		})
	})

	t.When("suite Config has invalid signature with params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_InvalidSignature_WithParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for invalid SuiteConfig signature")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported signature")
		})
	})

	t.When("suite Config has wrong return type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_InvalidSignature_WrongReturnType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong SuiteConfig return type")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported return type")
		})
	})

	t.When("parallel is parsed", func(w *gotest.T) {
		w.It("detects IsMethodParallel", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ParallelParsed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
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

	t.When("single-statement preset call", func(w *gotest.T) {
		w.It("accepts the gotest preset and reports not parallel", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_PresetCall")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.True(it, result.Suites[0].HasConfig(), "expected HasConfig() to be true")
			gotest.False(it, result.Suites[0].IsMethodParallel(), "presets leave Parallel false")
		})
	})

	t.When("single-statement unknown helper call", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_UnknownHelperCall_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for non-preset helper call")
			gotest.ErrorContains(it, result.Errs[0].Err, "only the gotest presets")
		})
	})

	t.When("a non-gotest function shadows a preset name", func(w *gotest.T) {
		w.It("reports an error — the name alone must not pass as a preset", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ForeignPresetName_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "a shadowing DefaultSuiteConfig returning Parallel: true would silently generate a sequential suite")
			gotest.ErrorContains(it, result.Errs[0].Err, "only the gotest presets")
		})
	})

	t.When("positional literal", func(w *gotest.T) {
		w.It("reports an error — Parallel in positional form is invisible to the static scan", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_PositionalLiteral_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for keyless SuiteConfig literal")
			gotest.ErrorContains(it, result.Errs[0].Err, "keyed fields")
		})
	})

	t.When("suite composes its config from a literal base", func(w *gotest.T) {
		w.It("detects Parallel from the base literal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ComposeLiteralBase")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.True(it, result.Suites[0].IsMethodParallel(), "Parallel: true in the compose base literal must be detected")
		})
	})

	t.When("compose form with unknown base call", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ComposeUnknownBase_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for non-preset compose base")
			gotest.ErrorContains(it, result.Errs[0].Err, "only the gotest presets")
		})
	})

	t.When("Parallel is assigned a non-literal value", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ParallelNonLiteral_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for cfg.Parallel = <var>")
			gotest.ErrorContains(it, result.Errs[0].Err, "boolean literal")
		})
	})

	t.When("Exclusive appears in the literal", func(w *gotest.T) {
		w.It("is resolved statically like Parallel", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ExclusiveParsed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.True(it, result.Suites[0].IsExclusive(), "Exclusive: true in the literal must be detected")
			gotest.False(it, result.Suites[0].IsMethodParallel())
		})
	})

	t.When("Exclusive is assigned in the compose form over a preset", func(w *gotest.T) {
		w.It("is resolved statically", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ExclusiveCompose")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.True(it, result.Suites[0].IsExclusive(), "cfg.Exclusive = true over a preset must be detected")
		})
	})

	t.When("Parallel is overridden back to false", func(w *gotest.T) {
		w.It("reports not parallel", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_ParallelFalseOverride")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.False(it, result.Suites[0].IsMethodParallel(), "cfg.Parallel = false must override the base literal")
		})
	})

	t.When("compose form is structurally malformed", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteConfig_MalformedBody_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for body not ending in `return cfg`")
			gotest.ErrorContains(it, result.Errs[0].Err, "must return a gotest.SuiteConfig literal or preset call")
		})
	})
}

func (s *CollectorTestSuite) TestSuiteGuard(t *gotest.T) {
	t.When("suite has Guard method", func(w *gotest.T) {
		w.It("detects HasGuard", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_Detected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.True(it, result.Suites[0].HasGuard(), "expected HasGuard() to be true")
		})
	})

	t.When("suite has no Guard method", func(w *gotest.T) {
		w.It("reports HasGuard as false", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_AbsentIsFalse")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.False(it, result.Suites[0].HasGuard(), "expected HasGuard() to be false")
		})
	})

	t.When("Guard has invalid signature with params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_InvalidSignature_WithParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for invalid SuiteGuard signature")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported signature")
		})
	})

	t.When("Guard has wrong return type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteGuard_InvalidSignature_WrongReturnType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for wrong SuiteGuard return type")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported return type")
		})
	})
}

func (s *CollectorTestSuite) TestBeforeEach(t *gotest.T) {
	t.When("returning form", func(w *gotest.T) {
		w.It("detects HasReturn on BeforeEach", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BeforeEach_ReturningForm")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.Len(it, result.Suites, 1)

			be := result.Suites[0].BeforeEach()
			gotest.NotZero(it, be, "expected BeforeEach")
			gotest.True(it, be.HasReturn(), "expected BeforeEach to have return type")
		})
	})

	t.When("too many returns", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BeforeEach_TooManyReturns")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for 2 return values")
			gotest.ErrorContains(it, result.Errs[0].Err, "expected 0 or 1 return values")
		})
	})
}

func (s *CollectorTestSuite) TestAfterEach(t *gotest.T) {
	t.When("with context param", func(w *gotest.T) {
		w.It("detects HasContextParam on AfterEach", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_AfterEach_WithContextParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)

			ae := result.Suites[0].AfterEach()
			gotest.NotZero(it, ae, "expected AfterEach")
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
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.Len(it, result.Suites[0].TestCases(), 2)
			gotest.True(it, result.Suites[0].TestCases()[0].HasContextParam(), "expected TestOne to have context param")
			gotest.True(it, result.Suites[0].TestCases()[1].HasContextParam(), "expected TestTwo to have context param")
		})
	})

	t.When("async with context", func(w *gotest.T) {
		w.It("detects HasContextParam on async test method", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_TestMethod_AsyncWithContext")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
			gotest.Len(it, result.Suites[0].TestCases(), 1)
			gotest.True(it, result.Suites[0].TestCases()[0].HasContextParam(), "expected context param")
		})
	})

	t.When("async method's last param is not done func()", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_TestMethod_AsyncWrongDoneParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for Async method without trailing done func()")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported last argument")
		})
	})

	t.When("too many params", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_TestMethod_TooManyParams")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for 3 params on a non-async method")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported number of params")
		})
	})
}

func (s *CollectorTestSuite) TestStdlibT(t *gotest.T) {
	t.When("suite detected", func(w *gotest.T) {
		w.It("detects stdlib T suite and UsesStdlibT", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_SuiteDetected")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.Equal(it, "PlainTestSuite", result.Suites[0].Identifier())
			gotest.Len(it, result.Suites[0].TestCases(), 1)
			gotest.True(it, result.Suites[0].TestCases()[0].UsesStdlibT(), "expected UsesStdlibT for *testing.T method")
		})
	})

	t.When("lifecycle hooks", func(w *gotest.T) {
		w.It("detects UsesStdlibT on all lifecycle hooks", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_LifecycleHooks")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)

			suite := result.Suites[0]
			gotest.NotZero(it, suite.BeforeAll(), "expected BeforeAll")
			gotest.True(it, suite.BeforeAll().UsesStdlibT(), "expected BeforeAll UsesStdlibT")
			gotest.NotZero(it, suite.AfterAll(), "expected AfterAll")
			gotest.True(it, suite.AfterAll().UsesStdlibT(), "expected AfterAll UsesStdlibT")
			gotest.NotZero(it, suite.BeforeEach(), "expected BeforeEach")
			gotest.True(it, suite.BeforeEach().UsesStdlibT(), "expected BeforeEach UsesStdlibT")
			gotest.NotZero(it, suite.AfterEach(), "expected AfterEach")
			gotest.True(it, suite.AfterEach().UsesStdlibT(), "expected AfterEach UsesStdlibT")
		})
	})

	t.When("mixed method signatures", func(w *gotest.T) {
		w.It("detects mixed stdlib and gotest T usage", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_MixedMethodSignatures")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites, 1)
			gotest.Len(it, result.Suites[0].TestCases(), 2)

			cases := result.Suites[0].TestCases()
			gotest.Equal(it, "TestStdlib", cases[0].Identifier())
			gotest.True(it, cases[0].UsesStdlibT(), "expected TestStdlib UsesStdlibT")
			gotest.Equal(it, "TestGotest", cases[1].Identifier())
			gotest.False(it, cases[1].UsesStdlibT(), "expected TestGotest NOT UsesStdlibT")
		})
	})

	t.When("wrong param type", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_StdlibT_WrongParamType")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for unsupported param type")
			gotest.ErrorContains(it, result.Errs[0].Err, "must be *gotest.T or *testing.T")
		})
	})

	t.When("test method uses *gotest.T", func(w *gotest.T) {
		w.It("reports NOT UsesStdlibT", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_GotestT_NotUsesStdlibT")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)
			gotest.Len(it, result.Suites[0].TestCases(), 1)
			gotest.False(it, result.Suites[0].TestCases()[0].UsesStdlibT(), "expected NOT UsesStdlibT for *gotest.T")
		})
	})
}

func (s *CollectorTestSuite) TestNilPackage(t *gotest.T) {
	t.When("CollectSuiteSpecs receives nil", func(w *gotest.T) {
		w.It("returns empty result", func(it *gotest.T) {
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(nil)
			gotest.Empty(it, result.Errs)
			gotest.Empty(it, result.Suites, "expected nil suites")
			gotest.Empty(it, result.Fixtures, "expected nil fixtures")
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
			gotest.ErrorContains(it, result.Errs[0].Err, "Parallel")
		})
	})

	t.When("parallel without BeforeEach", func(w *gotest.T) {
		w.It("is allowed for a field-less suite", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ParallelWithoutBeforeEach_Allowed")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "parallel with no BeforeEach should be allowed")
		})

		w.It("is allowed for suites with fields (write-once-in-BeforeAll is a valid pattern)", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ParallelStateNoBeforeEach")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "structural field checks cannot distinguish read-only from racy state")
		})

		w.It("is allowed when the only fields are fixture pointers", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ParallelFixtureOnlyNoBeforeEach")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "fixture pointer fields are wired once and read-only")
		})
	})

	t.When("unexported suite", func(w *gotest.T) {
		w.It("errors when it has test cases", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_UnexportedSuiteWithCases")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "unexported suite with cases is a silent false-green")
			gotest.ErrorContains(it, result.Errs[0].Err, "must be exported")
		})

		w.It("is allowed as a case-less embed base", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_UnexportedCaselessBase")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "helper/base types ending in TestSuite must stay legal")
		})
	})

	t.When("a referenced fixture has a value-receiver hook", func(w *gotest.T) {
		w.It("errors at resolution, not collection — incidental Fixture-named types stay legal", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Fixture_ValueReceiverHook")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "collection must not fail on value-receiver hooks")

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.ErrorContains(it, err, "value type receiver")
		})
	})

	t.When("method missing context param", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_MethodMissingContextParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: TestTwo missing context param")
			gotest.ErrorContains(it, result.Errs[0].Err, "TestTwo")
		})
	})

	t.When("AfterEach missing context param", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_AfterEachMissingContextParam")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: AfterEach missing context param")
			gotest.ErrorContains(it, result.Errs[0].Err, "AfterEach")
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
			gotest.ErrorContains(it, result.Errs[0].Err, "does not match")
		})
	})

	t.When("returning BeforeEach fully consistent", func(w *gotest.T) {
		w.It("reports no errors", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ReturningBeforeEach_FullyConsistent_OK")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)
		})
	})

	t.When("context must be pointer", func(w *gotest.T) {
		w.It("reports an error for non-pointer context", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_ContextMustBePointer")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: non-pointer context")
			gotest.ErrorContains(it, result.Errs[0].Err, "must be a pointer")
		})
	})

	t.When("orphan context test method", func(w *gotest.T) {
		w.It("reports an error when BeforeEach does not return a context", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_OrphanContextTestMethod")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: context param without returning BeforeEach")
			gotest.ErrorContains(it, result.Errs[0].Err, "BeforeEach does not return a context")
		})
	})

	t.When("AfterEach context type mismatch", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_Validation_AfterEachContextTypeMismatch")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: AfterEach context type mismatch")
			gotest.ErrorContains(it, result.Errs[0].Err, "AfterEach: context type")
		})
	})

	t.When("suite method has a value receiver", func(w *gotest.T) {
		w.It("reports an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_SuiteMethod_ValueReceiver")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for value-receiver suite method")
			gotest.ErrorContains(it, result.Errs[0].Err, "unsupported value type receiver")
		})
	})
}

func (s *CollectorTestSuite) TestBenchmarkMethod(t *gotest.T) {
	t.When("suite has benchmark methods", func(w *gotest.T) {
		w.It("classifies benchmark methods and applies X_ exclusion", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BenchmarkMethod")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs, "expected no errors, got: %v", result.Errs)

			spec := gotest.Must(c.ApplyTestSuiteSpecs(result))
			gotest.Len(it, spec.EffectiveTestSuites, 1)

			suite := spec.EffectiveTestSuites[0]
			gotest.Len(it, suite.Benchmarks(), 1)
			gotest.Equal(it, "BenchmarkParse", suite.Benchmarks()[0].Identifier())
		})
	})

	t.When("benchmark method has an unsupported signature", func(w *gotest.T) {
		w.It("rejects benchmark methods without *gotest.B/*testing.B", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BenchmarkMethod_BadSignature")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error for unsupported benchmark param type")
			gotest.ErrorContains(it, result.Errs[0].Err, "must accept exactly one parameter of type *gotest.B or *testing.B")
		})
	})

	t.When("suite has benchmarks and a returning BeforeEach", func(w *gotest.T) {
		w.It("rejects benchmarks on returning-BeforeEach suites", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestCollector_BenchmarkMethod_ReturningBeforeEach")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.NotEmpty(it, result.Errs, "expected error: benchmarks with returning BeforeEach")
			gotest.ErrorContains(it, result.Errs[0].Err, "move benchmarks to a dedicated suite")
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
			gotest.Empty(it, spec.EffectiveTestSuites, "expected no suites")
		})
	})
}
