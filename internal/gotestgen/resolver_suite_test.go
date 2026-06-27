package gotestgen_test

import (
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// ResolverTestSuite tests fixture binding resolution, including suite-to-fixture,
// parent-child, shared fixture, and lifecycle method detection.
type ResolverTestSuite struct{}

func (s *ResolverTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *ResolverTestSuite) TestIsInternalPkgPath(t *gotest.T) {
	t.When("various paths", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Desc     string
			path     string
			expected bool
		}{
			{"internal segment", "github.com/foo/internal/bar", true},
			{"internal leaf", "github.com/foo/internal", true},
			{"internal root", "internal/bar", true},
			{"no internal", "github.com/foo/bar", false},
			{"internalize prefix", "github.com/foo/internalize", false},
			{"pkg path", "github.com/foo/pkg/bar", false},
		}) {
			gotest.Equal(sub, tc.expected, gotestgen.ExportIsInternalPkgPath(tc.path))
		}
	})
}

func (s *ResolverTestSuite) TestSuiteToFixtureBinding(t *gotest.T) {
	t.When("suite embeds fixture", func(w *gotest.T) {
		w.It("resolves fixture binding via embedding", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_Embedding_SuiteToFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			gotest.Equal(it, "DBFixture", resolved.RootFixtures[0].Identifier)

			gotest.Len(it, resolved.FixtureBound, 1)
			gotest.Equal(it, "QueryTestSuite", resolved.FixtureBound[0].Identifier())
			gotest.Equal(it, "DBFixture", resolved.FixtureBound[0].FixtureFieldName())
		})
	})

	t.When("suite uses named field for fixture", func(w *gotest.T) {
		w.It("resolves fixture binding via named field", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NamedField_SuiteToFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			gotest.Equal(it, "DBFixture", resolved.RootFixtures[0].Identifier)

			gotest.Len(it, resolved.FixtureBound, 1)
			gotest.Equal(it, "QueryTestSuite", resolved.FixtureBound[0].Identifier())
			gotest.Equal(it, "db", resolved.FixtureBound[0].FixtureFieldName())
		})
	})
}

func (s *ResolverTestSuite) TestChildToParentFixtureBinding(t *gotest.T) {
	t.When("child embeds parent fixture", func(w *gotest.T) {
		w.It("resolves parent-child fixture hierarchy", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_Embedding_ChildToParentFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			root := resolved.RootFixtures[0]
			gotest.Equal(it, "InfraFixture", root.Identifier)
			gotest.Len(it, root.Children, 1)
			child := root.Children[0]
			gotest.Equal(it, "APIFixture", child.Identifier)
			gotest.Equal(it, "InfraFixture", child.ParentFieldName)
		})
	})

	t.When("child uses named field for parent", func(w *gotest.T) {
		w.It("resolves parent field name correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NamedField_ChildToParentFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			root := resolved.RootFixtures[0]
			gotest.Equal(it, "InfraFixture", root.Identifier)
			gotest.Len(it, root.Children, 1)
			child := root.Children[0]
			gotest.Equal(it, "APIFixture", child.Identifier)
			gotest.Equal(it, "infra", child.ParentFieldName)
		})
	})
}

func (s *ResolverTestSuite) TestSharedFixtureResolution(t *gotest.T) {
	t.When("fixture references shared fixture via named field", func(w *gotest.T) {
		w.It("resolves shared fixture reference", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NamedField_FixtureToSharedFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			root := resolved.RootFixtures[0]
			gotest.Len(it, root.SharedFixtures, 1)
			gotest.Equal(it, "pg", root.SharedFixtures[0].FieldName)
			gotest.Equal(it, "PGSharedFixture", root.SharedFixtures[0].QualifiedType)

			gotest.Len(it, resolved.RequiredSharedFixtures, 1)
			gotest.Equal(it, "PGSharedFixture", resolved.RequiredSharedFixtures[0].Identifier)
		})
	})

	t.When("suite directly embeds shared fixture", func(w *gotest.T) {
		w.It("resolves as standalone with shared fixture reference", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_DirectSharedFixture_Embedded")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Empty(it, resolved.RootFixtures, "no package fixture")
			gotest.Empty(it, resolved.FixtureBound, "suite not fixture-bound")
			gotest.Len(it, resolved.Standalone, 1, "suite is standalone")
			gotest.Equal(it, "UserTestSuite", resolved.Standalone[0].Identifier())

			gotest.NotEmpty(it, resolved.SuiteSharedFixtures, "SuiteSharedFixtures should be populated")
			refs := resolved.SuiteSharedFixtures["UserTestSuite"]
			gotest.Len(it, refs, 1)
			gotest.Equal(it, "PGSharedFixture", refs[0].FieldName)
			gotest.Equal(it, "PGSharedFixture", refs[0].QualifiedType)
			gotest.Equal(it, "sf0", refs[0].LocalVar)

			gotest.Len(it, resolved.RequiredSharedFixtures, 1)
			gotest.Equal(it, "PGSharedFixture", resolved.RequiredSharedFixtures[0].Identifier)
		})
	})

	t.When("suite uses named field for shared fixture", func(w *gotest.T) {
		w.It("resolves field name correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_DirectSharedFixture_NamedField")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Empty(it, resolved.RootFixtures)
			gotest.Len(it, resolved.Standalone, 1)

			refs := resolved.SuiteSharedFixtures["UserTestSuite"]
			gotest.Len(it, refs, 1)
			gotest.Equal(it, "pg", refs[0].FieldName)
		})
	})

	t.When("suite has both fixture and direct shared fixture", func(w *gotest.T) {
		w.It("resolves both bindings", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SuiteWithFixtureAndDirectSharedFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1, "package fixture found")
			gotest.Len(it, resolved.FixtureBound, 1, "suite is fixture-bound")
			gotest.Equal(it, "AppFixture", resolved.FixtureBound[0].FixtureFieldName())

			refs := resolved.SuiteSharedFixtures["UserTestSuite"]
			gotest.Len(it, refs, 1, "direct shared fixture also recorded")
			gotest.Equal(it, "PGSharedFixture", refs[0].FieldName)

			gotest.Len(it, resolved.RequiredSharedFixtures, 1)
		})
	})

	t.When("suite references multiple shared fixtures directly", func(w *gotest.T) {
		w.It("resolves all shared fixture references", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_DirectMultipleSharedFixtures_OnSuite")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Empty(it, resolved.RootFixtures)
			gotest.Len(it, resolved.Standalone, 1)

			refs := resolved.SuiteSharedFixtures["FullTestSuite"]
			gotest.Len(it, refs, 2)

			fieldNames := map[string]string{}
			for _, ref := range refs {
				fieldNames[ref.QualifiedType] = ref.FieldName
			}
			gotest.Equal(it, "pg", fieldNames["PGSharedFixture"])
			gotest.Equal(it, "redis", fieldNames["RedisSharedFixture"])

			gotest.Len(it, resolved.RequiredSharedFixtures, 2)
		})
	})

	t.When("shared fixture has transfer fields", func(w *gotest.T) {
		w.It("resolves transfer fields correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SharedFixture_TransferFields")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RequiredSharedFixtures, 1)
			sf := resolved.RequiredSharedFixtures[0]
			gotest.Equal(it, "PGSharedFixture", sf.Identifier)
			gotest.Contains(it, sf.TransferFields, "ConnStr")
			gotest.Contains(it, sf.TransferFields, "Port")
			gotest.False(it, sf.HasHydrate, "no Hydrate method")
			gotest.False(it, sf.HasDehydrate, "no Dehydrate method")
		})
	})
}

func (s *ResolverTestSuite) TestResolutionErrors(t *gotest.T) {
	t.When("fixtures form a cycle", func(w *gotest.T) {
		w.It("returns an error mentioning cycle", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_CycleDetection")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.Error(it, err)
			gotest.ErrorContains(it, err, "cycle")
		})
	})

	t.When("suite references multiple fixtures", func(w *gotest.T) {
		w.It("resolves all fixtures", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MultipleFixturesPerSuite")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)
			gotest.Len(it, resolved.RootFixtures, 2)
			gotest.Len(it, resolved.FixtureBound, 1)

			bindings := resolved.SuiteFixtureFields["MultiTestSuite"]
			gotest.Len(it, bindings, 2)
		})
	})

	t.When("fixture lacks BeforeAll", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MissingBeforeAll")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.Error(it, err)
			gotest.ErrorContains(it, err, "must have a BeforeAll")
		})
	})

	t.When("fixture has multiple parent fixtures", func(w *gotest.T) {
		w.It("resolves all parent fixtures", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MultipleParentFixtures")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)
			gotest.Len(it, resolved.RootFixtures, 2, "A and B should be roots")
			gotest.Len(it, resolved.FixtureBound, 1)
		})
	})

	t.When("diamond dependency", func(w *gotest.T) {
		w.It("deduplicates shared ancestor", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_DiamondDependency")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)
			gotest.Len(it, resolved.RootFixtures, 1, "DB should be the only root")
			gotest.Len(it, resolved.AllFixtures, 3)
		})
	})

	t.When("shared fixture has non-serializable transfer field", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SharedFixture_NonSerializableField")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.Error(it, err, "expected error for non-serializable transfer field")
			gotest.ErrorContains(it, err, "non-JSON-serializable")
			gotest.ErrorContains(it, err, "channel")
		})
	})
}

func (s *ResolverTestSuite) TestMixedFieldStylesSameFixture(t *gotest.T) {
	t.When("two suites reference the same fixture with different field styles", func(w *gotest.T) {
		w.It("resolves each with its own field name", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MixedFieldStyles_SameFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			gotest.Len(it, resolved.FixtureBound, 2)

			fieldNames := map[string]string{}
			for _, s := range resolved.FixtureBound {
				fieldNames[s.Identifier()] = s.FixtureFieldName()
			}
			gotest.Equal(it, "DBFixture", fieldNames["EmbeddedTestSuite"])
			gotest.Equal(it, "db", fieldNames["NamedTestSuite"])
		})
	})
}

func (s *ResolverTestSuite) TestNoFixtureStandalone(t *gotest.T) {
	t.When("no fixture is defined", func(w *gotest.T) {
		w.It("resolves suite as standalone", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NoFixture_Standalone")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Empty(it, resolved.RootFixtures)
			gotest.Empty(it, resolved.FixtureBound)
			gotest.Len(it, resolved.Standalone, 1)
			gotest.Equal(it, "PlainTestSuite", resolved.Standalone[0].Identifier())
		})
	})
}

func (s *ResolverTestSuite) TestLifecycleMethodsDetection(t *gotest.T) {
	t.When("fixture has all lifecycle methods", func(w *gotest.T) {
		w.It("detects all lifecycle methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_LifecycleMethods_Detection")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			rf := resolved.RootFixtures[0]
			gotest.True(it, rf.BeforeAll, "BeforeAll detected")
			gotest.True(it, rf.AfterAll, "AfterAll detected")
			gotest.True(it, rf.BeforeEach, "BeforeEach detected")
			gotest.True(it, rf.AfterEach, "AfterEach detected")
		})
	})
}

func (s *ResolverTestSuite) TestUnreferencedFixtureNotInOutput(t *gotest.T) {
	t.When("a fixture is not referenced by any suite", func(w *gotest.T) {
		w.It("excludes the unreferenced fixture from output", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_UnreferencedFixture_NotInOutput")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RootFixtures, 1)
			gotest.Equal(it, "UsedFixture", resolved.RootFixtures[0].Identifier)
		})
	})
}

func (s *ResolverTestSuite) TestGenericAlias(t *gotest.T) {
	t.When("generic alias in pxtest package", func(w *gotest.T) {
		w.It("rejects generic alias", func(it *gotest.T) {
			suite := gotestast.NewTestSuiteSpecForTest("FooTestSuite", "mypkg_test", true)
			pkg := &packages.Package{Name: "mypkg_test", PkgPath: "example.com/mypkg_test"}

			_, err := gotestgen.Resolve(pkg, []*gotestast.TestSuiteSpec{suite}, nil)
			gotest.Error(it, err, "expected error for generic alias in pxtest")
			gotest.ErrorContains(it, err, "must not be in an external test package")
		})
	})

	t.When("generic alias in internal test package", func(w *gotest.T) {
		w.It("allows generic alias", func(it *gotest.T) {
			suite := gotestast.NewTestSuiteSpecForTest("FooTestSuite", "mypkg", true)
			pkg := &packages.Package{Name: "mypkg", PkgPath: "example.com/mypkg"}

			_, err := gotestgen.Resolve(pkg, []*gotestast.TestSuiteSpec{suite}, nil)
			gotest.NoError(it, err)
		})
	})
}

func (s *ResolverTestSuite) TestSharedFixtureDependencies(t *gotest.T) {
	t.When("shared fixture depends on another shared fixture", func(w *gotest.T) {
		w.It("resolves the dependency chain", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SharedFixture_DependsOnSharedFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Len(it, resolved.RequiredSharedFixtures, 2)

			// Find Schema and verify it has PG as a dependency
			var schema *gotestgen.SharedFixtureInfo
			for i := range resolved.RequiredSharedFixtures {
				if resolved.RequiredSharedFixtures[i].Identifier == "SchemaSharedFixture" {
					schema = &resolved.RequiredSharedFixtures[i]
				}
			}
			gotest.NotZero(it, schema, "expected SchemaSharedFixture in required list")
			gotest.Len(it, schema.Dependencies, 1)
			gotest.Contains(it, schema.Dependencies[0], "PGSharedFixture")

			gotest.NotContains(it, schema.TransferFields, "PG", "dep pointer field excluded from transfer")
			gotest.Contains(it, schema.TransferFields, "Version", "non-dep exported field is a transfer field")
		})
	})

	t.When("suite has transitive shared fixture dependencies", func(w *gotest.T) {
		w.It("computes full required set including transitive deps", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SharedFixture_TransitiveDeps")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Empty(it, result.Errs)

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			// UserTestSuite references Schema which depends on PG → needs both
			userKeys := resolved.SuiteRequiredSharedFixtureKeys["UserTestSuite"]
			gotest.Len(it, userKeys, 2, "UserTestSuite needs PG + Schema")

			// SimpleTestSuite references only PG → needs only PG
			simpleKeys := resolved.SuiteRequiredSharedFixtureKeys["SimpleTestSuite"]
			gotest.Len(it, simpleKeys, 1, "SimpleTestSuite needs only PG")
		})
	})
}
