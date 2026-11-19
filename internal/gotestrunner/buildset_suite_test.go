package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BuildSetTestSuite: a run builds the packages whose suites it dispatches,
// and only those.
type BuildSetTestSuite struct{}

func (s *BuildSetTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *BuildSetTestSuite) TestBuildSet(t *gotest.T) {
	pkgs := []string{"m/a", "m/b", "m/c"}
	t.It("keeps the dispatched packages in report order", func(it *gotest.T) {
		gotest.Equal(it, []string{"m/a", "m/c"}, gotestrunner.ExportBuildSet(pkgs, map[string]bool{"m/c": true, "m/a": true}))
	})
	t.It("is empty when nothing is dispatched", func(it *gotest.T) {
		gotest.Empty(it, gotestrunner.ExportBuildSet(pkgs, nil))
	})
}

func (s *BuildSetTestSuite) TestDispatchedPackages(t *gotest.T) {
	type row struct {
		Desc      string
		Run, Skip string
		Group     string // the suite's group
		SkipGroup string // --skip-group
		Fuzz      bool   // a testless suite with one fuzz wrapper
		Want      bool
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "no filter", Want: true},
		{Desc: "run enters the suite", Run: "^TestPTestSuite$", Want: true},
		{Desc: "run enters it partly", Run: "TestPTestSuite/TestX", Want: true},
		{Desc: "run names another suite", Run: "^TestOtherTestSuite$"},
		{Desc: "skip drops it whole", Skip: "^TestPTestSuite$"},
		{Desc: "skip drops a method only", Skip: "TestPTestSuite/TestX", Want: true},
		{Desc: "its group is left out", Group: "e2e", SkipGroup: "e2e"},
		{Desc: "a fuzz wrapper is named", Run: "^FuzzPTestSuite_Seed$", Fuzz: true, Want: true},
		{Desc: "run does not compile", Run: "(", Want: true},
	}) {
		suite := gotestrunner.PlannedSuite{Package: "m/p", Name: "PTestSuite", Tests: true, Group: r.Group}
		if r.Fuzz {
			suite.Testless = true
			suite.FuzzFuncs = []gotestrunner.PlannedFuzz{{Func: "FuzzPTestSuite_Seed"}}
		}
		var skipGroups []string
		if r.SkipGroup != "" {
			skipGroups = []string{r.SkipGroup}
		}
		got := gotestrunner.ExportDispatchedPackages(gotestrunner.ExportPlannedOverlay(nil, suite), r.Run, r.Skip, skipGroups)
		gotest.Equal(t, r.Want, got["m/p"])
	}
}
