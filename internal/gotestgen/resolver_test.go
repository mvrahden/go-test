package gotestgen //nolint:stdlib-test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestIsInternalPkgPath(t *testing.T) {
	t.Parallel()
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
		gotest.Equal(t, tc.expected, isInternalPkgPath(tc.path), tc.path)
	}
}

func TestResolve_Embedding_SuiteToFixture(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	gotest.Equal(t, "DBFixture", resolved.RootFixtures[0].Identifier)

	gotest.Equal(t, 1, len(resolved.FixtureBound))
	gotest.Equal(t, "QueryTestSuite", resolved.FixtureBound[0].Identifier())
	gotest.Equal(t, "DBFixture", resolved.FixtureBound[0].FixtureFieldName())
}

func TestResolve_NamedField_SuiteToFixture(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	gotest.Equal(t, "DBFixture", resolved.RootFixtures[0].Identifier)

	gotest.Equal(t, 1, len(resolved.FixtureBound))
	gotest.Equal(t, "QueryTestSuite", resolved.FixtureBound[0].Identifier())
	gotest.Equal(t, "db", resolved.FixtureBound[0].FixtureFieldName())
}

func TestResolve_Embedding_ChildToParentFixture(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	root := resolved.RootFixtures[0]
	gotest.Equal(t, "InfraFixture", root.Identifier)
	gotest.Equal(t, 1, len(root.Children))
	child := root.Children[0]
	gotest.Equal(t, "APIFixture", child.Identifier)
	gotest.Equal(t, "InfraFixture", child.ParentFieldName)
}

func TestResolve_NamedField_ChildToParentFixture(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	root := resolved.RootFixtures[0]
	gotest.Equal(t, "InfraFixture", root.Identifier)
	gotest.Equal(t, 1, len(root.Children))
	child := root.Children[0]
	gotest.Equal(t, "APIFixture", child.Identifier)
	gotest.Equal(t, "infra", child.ParentFieldName)
}

func TestResolve_NamedField_FixtureToSharedFixture(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	root := resolved.RootFixtures[0]
	gotest.Equal(t, 1, len(root.SharedFixtures))
	gotest.Equal(t, "pg", root.SharedFixtures[0].FieldName)
	gotest.Equal(t, "PGSharedFixture", root.SharedFixtures[0].QualifiedType)

	gotest.Equal(t, 1, len(resolved.RequiredSharedFixtures))
	gotest.Equal(t, "PGSharedFixture", resolved.RequiredSharedFixtures[0].Identifier)
}

func TestResolve_DirectSharedFixture_Embedded(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 0, len(resolved.RootFixtures), "no package fixture")
	gotest.Equal(t, 0, len(resolved.FixtureBound), "suite not fixture-bound")
	gotest.Equal(t, 1, len(resolved.Standalone), "suite is standalone")
	gotest.Equal(t, "UserTestSuite", resolved.Standalone[0].Identifier())

	gotest.True(t, resolved.SuiteSharedFixtures != nil, "SuiteSharedFixtures should be populated")
	refs := resolved.SuiteSharedFixtures["UserTestSuite"]
	gotest.Equal(t, 1, len(refs))
	gotest.Equal(t, "PGSharedFixture", refs[0].FieldName)
	gotest.Equal(t, "PGSharedFixture", refs[0].QualifiedType)
	gotest.Equal(t, "sf0", refs[0].LocalVar)

	gotest.Equal(t, 1, len(resolved.RequiredSharedFixtures))
	gotest.Equal(t, "PGSharedFixture", resolved.RequiredSharedFixtures[0].Identifier)
}

func TestResolve_DirectSharedFixture_NamedField(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 0, len(resolved.RootFixtures))
	gotest.Equal(t, 1, len(resolved.Standalone))

	refs := resolved.SuiteSharedFixtures["UserTestSuite"]
	gotest.Equal(t, 1, len(refs))
	gotest.Equal(t, "pg", refs[0].FieldName)
}

func TestResolve_SuiteWithFixtureAndDirectSharedFixture(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures), "package fixture found")
	gotest.Equal(t, 1, len(resolved.FixtureBound), "suite is fixture-bound")
	gotest.Equal(t, "AppFixture", resolved.FixtureBound[0].FixtureFieldName())

	refs := resolved.SuiteSharedFixtures["UserTestSuite"]
	gotest.Equal(t, 1, len(refs), "direct shared fixture also recorded")
	gotest.Equal(t, "PGSharedFixture", refs[0].FieldName)

	gotest.Equal(t, 1, len(resolved.RequiredSharedFixtures))
}

func TestResolve_MixedFieldStyles_SameFixture(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	gotest.Equal(t, 2, len(resolved.FixtureBound))

	fieldNames := map[string]string{}
	for _, s := range resolved.FixtureBound {
		fieldNames[s.Identifier()] = s.FixtureFieldName()
	}
	gotest.Equal(t, "DBFixture", fieldNames["EmbeddedTestSuite"])
	gotest.Equal(t, "db", fieldNames["NamedTestSuite"])
}

func TestResolve_CycleDetection(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	_, err = Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "cycle")
}

func TestResolve_MultipleFixturesPerSuite(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	_, err = Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "multiple fixtures")
}

func TestResolve_MissingBeforeAll(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	_, err = Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "must have a BeforeAll")
}

func TestResolve_NoFixture_Standalone(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 0, len(resolved.RootFixtures))
	gotest.Equal(t, 0, len(resolved.FixtureBound))
	gotest.Equal(t, 1, len(resolved.Standalone))
	gotest.Equal(t, "PlainTestSuite", resolved.Standalone[0].Identifier())
}

func TestResolve_MultipleParentFixtures_Error(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	_, err = Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.Error(t, err)
	gotest.Contains(t, err.Error(), "multiple fixtures")
}

func TestResolve_SharedFixture_TransferFields(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RequiredSharedFixtures))
	sf := resolved.RequiredSharedFixtures[0]
	gotest.Equal(t, "PGSharedFixture", sf.Identifier)
	gotest.Contains(t, sf.TransferFields, "ConnStr")
	gotest.Contains(t, sf.TransferFields, "Port")
	gotest.True(t, !sf.HasHydrate, "no Hydrate method")
	gotest.True(t, !sf.HasDehydrate, "no Dehydrate method")
}

func TestResolve_LifecycleMethods_Detection(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	rf := resolved.RootFixtures[0]
	gotest.True(t, rf.BeforeAll, "BeforeAll detected")
	gotest.True(t, rf.AfterAll, "AfterAll detected")
	gotest.True(t, rf.BeforeEach, "BeforeEach detected")
	gotest.True(t, rf.AfterEach, "AfterEach detected")
}

func TestResolve_UnreferencedFixture_NotInOutput(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 1, len(resolved.RootFixtures))
	gotest.Equal(t, "UsedFixture", resolved.RootFixtures[0].Identifier)
}

func TestResolve_DirectMultipleSharedFixtures_OnSuite(t *testing.T) {
	t.Parallel()
	pkg := mustTestPkg(t)
	c := collector{}
	result := c.CollectSuiteSpecs(pkg)
	gotest.Equal(t, 0, len(result.Errs))

	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, result.Fixtures)
	gotest.NoError(t, err)

	gotest.Equal(t, 0, len(resolved.RootFixtures))
	gotest.Equal(t, 1, len(resolved.Standalone))

	refs := resolved.SuiteSharedFixtures["FullTestSuite"]
	gotest.Equal(t, 2, len(refs))

	fieldNames := map[string]string{}
	for _, ref := range refs {
		fieldNames[ref.QualifiedType] = ref.FieldName
	}
	gotest.Equal(t, "pg", fieldNames["PGSharedFixture"])
	gotest.Equal(t, "redis", fieldNames["RedisSharedFixture"])

	gotest.Equal(t, 2, len(resolved.RequiredSharedFixtures))
}
