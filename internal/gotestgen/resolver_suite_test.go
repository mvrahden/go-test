package gotestgen_test

import (
	"fmt"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

type ResolverTestSuite struct{}

func (s *ResolverTestSuite) TestIsInternalPkgPath(t *gotest.T) {
	t.When("various paths", func(w *gotest.T) {
		for _, tc := range []struct {
			path     string
			expected bool
		}{
			{"github.com/foo/internal/bar", true},
			{"github.com/foo/internal", true},
			{"internal/bar", true},
			{"github.com/foo/bar", false},
			{"github.com/foo/internalize", false},
			{"github.com/foo/pkg/bar", false},
		} {
			w.It("returns "+fmt.Sprintf("%v", tc.expected)+" for "+tc.path, func(it *gotest.T) {
				gotest.Equal(it, tc.expected, gotestgen.ExportIsInternalPkgPath(tc.path), tc.path)
			})
		}
	})
}

func (s *ResolverTestSuite) TestEmbeddingSuiteToFixture(t *gotest.T) {
	t.When("suite embeds fixture", func(w *gotest.T) {
		w.It("resolves fixture binding via embedding", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_Embedding_SuiteToFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
			gotest.Equal(it, "DBFixture", resolved.RootFixtures[0].Identifier)

			gotest.Equal(it, 1, len(resolved.FixtureBound))
			gotest.Equal(it, "QueryTestSuite", resolved.FixtureBound[0].Identifier())
			gotest.Equal(it, "DBFixture", resolved.FixtureBound[0].FixtureFieldName())
		})
	})
}

func (s *ResolverTestSuite) TestNamedFieldSuiteToFixture(t *gotest.T) {
	t.When("suite uses named field for fixture", func(w *gotest.T) {
		w.It("resolves fixture binding via named field", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NamedField_SuiteToFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
			gotest.Equal(it, "DBFixture", resolved.RootFixtures[0].Identifier)

			gotest.Equal(it, 1, len(resolved.FixtureBound))
			gotest.Equal(it, "QueryTestSuite", resolved.FixtureBound[0].Identifier())
			gotest.Equal(it, "db", resolved.FixtureBound[0].FixtureFieldName())
		})
	})
}

func (s *ResolverTestSuite) TestEmbeddingChildToParentFixture(t *gotest.T) {
	t.When("child fixture embeds parent fixture", func(w *gotest.T) {
		w.It("resolves parent-child fixture hierarchy", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_Embedding_ChildToParentFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
			root := resolved.RootFixtures[0]
			gotest.Equal(it, "InfraFixture", root.Identifier)
			gotest.Equal(it, 1, len(root.Children))
			child := root.Children[0]
			gotest.Equal(it, "APIFixture", child.Identifier)
			gotest.Equal(it, "InfraFixture", child.ParentFieldName)
		})
	})
}

func (s *ResolverTestSuite) TestNamedFieldChildToParentFixture(t *gotest.T) {
	t.When("child fixture uses named field for parent", func(w *gotest.T) {
		w.It("resolves parent field name correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NamedField_ChildToParentFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
			root := resolved.RootFixtures[0]
			gotest.Equal(it, "InfraFixture", root.Identifier)
			gotest.Equal(it, 1, len(root.Children))
			child := root.Children[0]
			gotest.Equal(it, "APIFixture", child.Identifier)
			gotest.Equal(it, "infra", child.ParentFieldName)
		})
	})
}

func (s *ResolverTestSuite) TestNamedFieldFixtureToSharedFixture(t *gotest.T) {
	t.When("fixture references shared fixture via named field", func(w *gotest.T) {
		w.It("resolves shared fixture reference", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NamedField_FixtureToSharedFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
			root := resolved.RootFixtures[0]
			gotest.Equal(it, 1, len(root.SharedFixtures))
			gotest.Equal(it, "pg", root.SharedFixtures[0].FieldName)
			gotest.Equal(it, "PGSharedFixture", root.SharedFixtures[0].QualifiedType)

			gotest.Equal(it, 1, len(resolved.RequiredSharedFixtures))
			gotest.Equal(it, "PGSharedFixture", resolved.RequiredSharedFixtures[0].Identifier)
		})
	})
}

func (s *ResolverTestSuite) TestDirectSharedFixtureEmbedded(t *gotest.T) {
	t.When("suite directly embeds shared fixture", func(w *gotest.T) {
		w.It("resolves as standalone with shared fixture reference", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_DirectSharedFixture_Embedded")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 0, len(resolved.RootFixtures), "no package fixture")
			gotest.Equal(it, 0, len(resolved.FixtureBound), "suite not fixture-bound")
			gotest.Equal(it, 1, len(resolved.Standalone), "suite is standalone")
			gotest.Equal(it, "UserTestSuite", resolved.Standalone[0].Identifier())

			gotest.True(it, resolved.SuiteSharedFixtures != nil, "SuiteSharedFixtures should be populated")
			refs := resolved.SuiteSharedFixtures["UserTestSuite"]
			gotest.Equal(it, 1, len(refs))
			gotest.Equal(it, "PGSharedFixture", refs[0].FieldName)
			gotest.Equal(it, "PGSharedFixture", refs[0].QualifiedType)
			gotest.Equal(it, "sf0", refs[0].LocalVar)

			gotest.Equal(it, 1, len(resolved.RequiredSharedFixtures))
			gotest.Equal(it, "PGSharedFixture", resolved.RequiredSharedFixtures[0].Identifier)
		})
	})
}

func (s *ResolverTestSuite) TestDirectSharedFixtureNamedField(t *gotest.T) {
	t.When("suite uses named field for shared fixture", func(w *gotest.T) {
		w.It("resolves field name correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_DirectSharedFixture_NamedField")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 0, len(resolved.RootFixtures))
			gotest.Equal(it, 1, len(resolved.Standalone))

			refs := resolved.SuiteSharedFixtures["UserTestSuite"]
			gotest.Equal(it, 1, len(refs))
			gotest.Equal(it, "pg", refs[0].FieldName)
		})
	})
}

func (s *ResolverTestSuite) TestSuiteWithFixtureAndDirectSharedFixture(t *gotest.T) {
	t.When("suite has both fixture and direct shared fixture", func(w *gotest.T) {
		w.It("resolves both bindings", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SuiteWithFixtureAndDirectSharedFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures), "package fixture found")
			gotest.Equal(it, 1, len(resolved.FixtureBound), "suite is fixture-bound")
			gotest.Equal(it, "AppFixture", resolved.FixtureBound[0].FixtureFieldName())

			refs := resolved.SuiteSharedFixtures["UserTestSuite"]
			gotest.Equal(it, 1, len(refs), "direct shared fixture also recorded")
			gotest.Equal(it, "PGSharedFixture", refs[0].FieldName)

			gotest.Equal(it, 1, len(resolved.RequiredSharedFixtures))
		})
	})
}

func (s *ResolverTestSuite) TestMixedFieldStylesSameFixture(t *gotest.T) {
	t.When("two suites reference the same fixture with different field styles", func(w *gotest.T) {
		w.It("resolves each with its own field name", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MixedFieldStyles_SameFixture")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
			gotest.Equal(it, 2, len(resolved.FixtureBound))

			fieldNames := map[string]string{}
			for _, s := range resolved.FixtureBound {
				fieldNames[s.Identifier()] = s.FixtureFieldName()
			}
			gotest.Equal(it, "DBFixture", fieldNames["EmbeddedTestSuite"])
			gotest.Equal(it, "db", fieldNames["NamedTestSuite"])
		})
	})
}

func (s *ResolverTestSuite) TestCycleDetection(t *gotest.T) {
	t.When("fixtures form a cycle", func(w *gotest.T) {
		w.It("returns an error mentioning cycle", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_CycleDetection")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.Error(it, err)
			gotest.Contains(it, err.Error(), "cycle")
		})
	})
}

func (s *ResolverTestSuite) TestMultipleFixturesPerSuite(t *gotest.T) {
	t.When("suite references multiple fixtures", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MultipleFixturesPerSuite")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.Error(it, err)
			gotest.Contains(it, err.Error(), "multiple fixtures")
		})
	})
}

func (s *ResolverTestSuite) TestMissingBeforeAll(t *gotest.T) {
	t.When("fixture lacks BeforeAll", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MissingBeforeAll")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.Error(it, err)
			gotest.Contains(it, err.Error(), "must have a BeforeAll")
		})
	})
}

func (s *ResolverTestSuite) TestNoFixtureStandalone(t *gotest.T) {
	t.When("no fixture is defined", func(w *gotest.T) {
		w.It("resolves suite as standalone", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_NoFixture_Standalone")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 0, len(resolved.RootFixtures))
			gotest.Equal(it, 0, len(resolved.FixtureBound))
			gotest.Equal(it, 1, len(resolved.Standalone))
			gotest.Equal(it, "PlainTestSuite", resolved.Standalone[0].Identifier())
		})
	})
}

func (s *ResolverTestSuite) TestMultipleParentFixturesError(t *gotest.T) {
	t.When("fixture has multiple parent fixtures", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_MultipleParentFixtures_Error")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.Error(it, err)
			gotest.Contains(it, err.Error(), "multiple fixtures")
		})
	})
}

func (s *ResolverTestSuite) TestSharedFixtureTransferFields(t *gotest.T) {
	t.When("shared fixture has transfer fields", func(w *gotest.T) {
		w.It("resolves transfer fields correctly", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SharedFixture_TransferFields")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RequiredSharedFixtures))
			sf := resolved.RequiredSharedFixtures[0]
			gotest.Equal(it, "PGSharedFixture", sf.Identifier)
			gotest.Contains(it, sf.TransferFields, "ConnStr")
			gotest.Contains(it, sf.TransferFields, "Port")
			gotest.True(it, !sf.HasHydrate, "no Hydrate method")
			gotest.True(it, !sf.HasDehydrate, "no Dehydrate method")
		})
	})
}

func (s *ResolverTestSuite) TestLifecycleMethodsDetection(t *gotest.T) {
	t.When("fixture has all lifecycle methods", func(w *gotest.T) {
		w.It("detects all lifecycle methods", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_LifecycleMethods_Detection")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
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
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 1, len(resolved.RootFixtures))
			gotest.Equal(it, "UsedFixture", resolved.RootFixtures[0].Identifier)
		})
	})
}

func (s *ResolverTestSuite) TestDirectMultipleSharedFixturesOnSuite(t *gotest.T) {
	t.When("suite references multiple shared fixtures directly", func(w *gotest.T) {
		w.It("resolves all shared fixture references", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_DirectMultipleSharedFixtures_OnSuite")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			resolved, err := gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.NoError(it, err)

			gotest.Equal(it, 0, len(resolved.RootFixtures))
			gotest.Equal(it, 1, len(resolved.Standalone))

			refs := resolved.SuiteSharedFixtures["FullTestSuite"]
			gotest.Equal(it, 2, len(refs))

			fieldNames := map[string]string{}
			for _, ref := range refs {
				fieldNames[ref.QualifiedType] = ref.FieldName
			}
			gotest.Equal(it, "pg", fieldNames["PGSharedFixture"])
			gotest.Equal(it, "redis", fieldNames["RedisSharedFixture"])

			gotest.Equal(it, 2, len(resolved.RequiredSharedFixtures))
		})
	})
}

func (s *ResolverTestSuite) TestSharedFixtureNonSerializableField(t *gotest.T) {
	t.When("shared fixture has non-serializable transfer field", func(w *gotest.T) {
		w.It("returns an error", func(it *gotest.T) {
			pkg := gotestgen.ExportMustTestPkg(it.T(), "TestResolve_SharedFixture_NonSerializableField")
			c := gotestgen.NewCollector()
			result := c.CollectSuiteSpecs(pkg)
			gotest.Equal(it, 0, len(result.Errs))

			spec, err := c.ApplyTestSuiteSpecs(result)
			gotest.NoError(it, err)

			_, err = gotestgen.Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
			gotest.True(it, err != nil, "expected error for non-serializable transfer field")
			gotest.Contains(it, err.Error(), "non-JSON-serializable")
			gotest.Contains(it, err.Error(), "channel")
		})
	})
}

func (s *ResolverTestSuite) TestGenericAlias(t *gotest.T) {
	t.When("generic alias in pxtest package", func(w *gotest.T) {
		w.It("rejects generic alias", func(it *gotest.T) {
			suite := gotestast.NewTestSuiteSpecForTest("FooTestSuite", "mypkg_test", true)
			pkg := &packages.Package{Name: "mypkg_test", PkgPath: "example.com/mypkg_test"}

			_, err := gotestgen.Resolve(pkg, []*gotestast.TestSuiteSpec{suite}, nil)
			gotest.True(it, err != nil, "expected error for generic alias in pxtest")
			gotest.Contains(it, err.Error(), "must not be in an external test package")
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
