package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzFixturesTestSuite: a fuzz session starts shared fixtures only for the
// targets whose suites bind them.
type FuzzFixturesTestSuite struct{}

func (s *FuzzFixturesTestSuite) TestNoTargetBindsAFixture(t *gotest.T) {
	t.When("the package declares a shared fixture no target's suite binds", func(w *gotest.T) {
		w.It("starts nothing and hands every target an empty environment", func(it *gotest.T) {
			overlay := &gotestrunner.OverlayResult{
				WorkDir:        it.TempDir(),
				SharedFixtures: []gotestgen.SharedFixtureInfo{{PkgPath: "example.com/p", Identifier: "DBSharedFixture"}},
				SuiteRequiredSharedFixtureKeys: map[string]map[string][]string{
					"example.com/p": {"TestOtherTestSuite": {"example.com/p.DBSharedFixture"}},
				},
			}
			target := gotestrunner.FuzzTarget{Package: "example.com/p", Func: "FuzzParseTestSuite_FuzzOne", Suite: "ParseTestSuite"}
			fixtures, err := gotestrunner.StartFuzzFixtures(it.Context(), overlay, []gotestrunner.FuzzTarget{target}, 0)
			gotest.NoError(it, err)
			gotest.Zero(it, fixtures)
			env, never := fixtures.Env(target)
			gotest.Empty(it, env)
			gotest.Empty(it, never)
			gotest.NoError(it, fixtures.Teardown())
		})
	})
}
