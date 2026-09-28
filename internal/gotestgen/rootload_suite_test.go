package gotestgen_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RootLoadTestSuite covers the run path's package load: roots are
// type-checked from source, their dependencies are read from export data, and
// a fixture declared in a dependency is still classified from its source.
type RootLoadTestSuite struct{}

func (s *RootLoadTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const fixtureBoundPkg = "github.com/mvrahden/go-test/tests/sharedfixture/fixturebound"

func (s *RootLoadTestSuite) TestRunPathLoad(t *gotest.T) {
	loaded, broken, err := gotestgen.LoadPackages([]string{fixtureBoundPkg}, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)
	gotest.Len(t, loaded, 1)

	t.It("leaves dependencies unparsed", func(it *gotest.T) {
		gotest.NotEmpty(it, loaded[0].Ptest.Imports)
		for path, imp := range loaded[0].Ptest.Imports {
			gotest.Empty(it, imp.Syntax, "dependency %s was loaded from source", path)
		}
	})
	t.It("classifies the fields a dependency's Hydrate assigns as local", func(it *gotest.T) {
		alpha := alphaFixture(it, loaded)
		gotest.Equal(it, []string{"DataPath"}, alpha.TransferFields)
		gotest.Equal(it, []string{"Handle"}, alpha.LocalFields)
	})
}

func (s *RootLoadTestSuite) TestDiscoveryLoad(t *gotest.T) {
	loaded, broken, err := gotestgen.LoadPackages([]string{fixtureBoundPkg}, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)

	t.It("classifies a dependency's fixture the way the run path does", func(it *gotest.T) {
		alpha := alphaFixture(it, loaded)
		gotest.Equal(it, []string{"DataPath"}, alpha.TransferFields)
		gotest.Equal(it, []string{"Handle"}, alpha.LocalFields)
	})
}

func alphaFixture(t *gotest.T, loaded []*gotestgen.LoadResult) gotestgen.SharedFixtureInfo {
	_, shared, err := gotestgen.GenerateFromLoaded(loaded)
	gotest.NoError(t, err)
	for i := range shared {
		if shared[i].Identifier == "AlphaSharedFixture" {
			return shared[i]
		}
	}
	gotest.Fail(t, "AlphaSharedFixture was not resolved")
	return gotestgen.SharedFixtureInfo{}
}
