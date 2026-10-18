package gotestast_test

import (
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// HookCallsTestSuite pins the rejection of hooks calling t.T() under the
// fuzz and benchmark wrappers.
type HookCallsTestSuite struct{}

func (s *HookCallsTestSuite) TestSeveralHooksCallT(t *gotest.T) {
	pkg := loadTestdataPkg(t.T(), "./testdata/hookcalls")
	suites := collectHarvestSuites(t.T(), pkg)
	gotest.Len(t, suites, 1)

	t.It("names every offending hook, in source order, at the first one", func(it *gotest.T) {
		for range 20 {
			err := gotestast.ValidateContextConsistency(suites[0])
			gotest.ErrorContains(it, err, "suite SeededTestSuite has fuzz methods but AfterEach, BeforeAll and AfterAll call t.T(), which is nil under them")
			gotest.Equal(it, 8, pkg.Fset.Position(gotestast.PosOf(err)).Line)
		}
	})
}
