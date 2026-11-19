package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunPlanTestSuite covers the run plan: one place for every static fact a
// run reads about its suites.
type RunPlanTestSuite struct{}

// planCtx holds the overlay cleanups a method registers.
type planCtx struct{ cleanups []func() }

func (s *RunPlanTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// BeforeAll isolates the temp dir: the overlays the methods generate sweep it.
func (s *RunPlanTestSuite) BeforeAll(t *gotest.T) { testkit.IsolateTempDir(t) }

// AfterAll has nothing to release: Setenv restores the temp dir.
func (s *RunPlanTestSuite) AfterAll(_ *gotest.T) {}

func (s *RunPlanTestSuite) BeforeEach(_ *gotest.T) *planCtx { return &planCtx{} }

func (s *RunPlanTestSuite) AfterEach(_ *gotest.T, c *planCtx) {
	for _, cleanup := range c.cleanups {
		cleanup()
	}
}

func (s *RunPlanTestSuite) TestEveryFactHasOnePlace(t *gotest.T, _ *planCtx) {
	plan := gotestrunner.ExportPlanRun(gotestgen.GenerateResults{{
		PkgPath: "example.com/a", AbsPath: "/src/a", PTest: []byte("x"), StdlibTestCount: 2,
		SuiteNames: []string{"OneTestSuite", "SeedsTestSuite"}, TestlessSuiteNames: []string{"SeedsTestSuite"},
		BenchSuiteNames:     []string{"OneTestSuite", "OnlyBenchTestSuite"},
		ExclusiveSuiteNames: []string{"OneTestSuite"}, GroupBySuite: map[string]string{"OneTestSuite": "e2e"},
		FuzzFuncsBySuite:               map[string][]string{"SeedsTestSuite": {"FuzzSeedsTestSuite_FuzzX"}},
		FuzzParamsByFunc:               map[string][]string{"FuzzSeedsTestSuite_FuzzX": {"string", "int"}},
		FuzzDecodedFuncs:               map[string]bool{"FuzzSeedsTestSuite_FuzzX": true},
		SkippedSuiteNames:              []string{"X_GoneTestSuite"},
		SuiteRequiredSharedFixtureKeys: map[string][]string{"TestOneTestSuite": {"example.com/f.B", "example.com/f.A"}},
	}, {PkgPath: "example.com/empty", AbsPath: "/src/empty"}})

	gotest.Len(t, plan.Packages, 2)
	a := plan.Package("example.com/a")
	gotest.Equal(t, gotestrunner.PlannedPackage{
		Path: "example.com/a", Dir: "/src/a", Harness: true, StdlibTests: 2, Skipped: []string{"X_GoneTestSuite"},
		Suites: []gotestrunner.PlannedSuite{
			{Package: "example.com/a", Dir: "/src/a", Name: "OneTestSuite", Tests: true, Bench: true, Exclusive: true, Group: "e2e",
				SharedKeys: []string{"example.com/f.A", "example.com/f.B"}},
			{Package: "example.com/a", Dir: "/src/a", Name: "SeedsTestSuite", Tests: true, Testless: true,
				FuzzFuncs: []gotestrunner.PlannedFuzz{{Func: "FuzzSeedsTestSuite_FuzzX", Params: []string{"string", "int"}, Decoded: true}}},
			{Package: "example.com/a", Dir: "/src/a", Name: "OnlyBenchTestSuite", Bench: true},
		},
	}, *a)
	gotest.False(t, plan.Package("example.com/empty").Harness)
	gotest.Zero(t, plan.Package("example.com/none"))
	gotest.Equal(t, "TestOneTestSuite", plan.Suite("example.com/a", "OneTestSuite").TestFunc())
	gotest.Equal(t, map[string]int{"example.com/a": 2}, plan.StdlibTests())
	gotest.True(t, plan.HasBenchmarks())
	gotest.True(t, plan.Fuzz("example.com/a", "FuzzSeedsTestSuite_FuzzX").Decoded)
}

func (s *RunPlanTestSuite) TestThePlanSaysWhatTheMapsSay(t *gotest.T, c *planCtx) {
	for _, dir := range []string{"groups", "groupfixtures", "fuzzing", "benching", "fixataildep"} {
		t.When("the package is "+dir, func(w *gotest.T) {
			pkgs, err := gotestgen.LoadRunPackages([]string{"../../tests/conformance/testdata/" + dir + "/"}, nil)
			gotest.NoError(w, err)
			overlay, cleanup, err := gotestrunner.GenerateOverlay(pkgs.Loaded, pkgs.Broken, false, true, false)
			gotest.NoError(w, err)
			c.cleanups = append(c.cleanups, cleanup)
			w.It("plans every suite as the maps do", func(it *gotest.T) {
				gotest.NotEmpty(it, overlay.Plan.All())
				gotest.Equal(it, gotestrunner.ExportPlanFromMaps(overlay).Packages, overlay.Plan.Packages)
			})
		})
	}
}
