package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureWindowTestSuite pins the alive-set math behind fixture window
// scheduling: which shared fixtures a run keeps resident, per phase, given
// the suites it will actually dispatch.
type FixtureWindowTestSuite struct{}

func (s *FixtureWindowTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// windowCtx holds the overlay cleanup a test registers, if any.
type windowCtx struct{ cleanup func() }

// BeforeAll isolates the temp dir for every method: the runs they make sweep it.
func (s *FixtureWindowTestSuite) BeforeAll(t *gotest.T) { testkit.IsolateTempDir(t) }

// AfterAll has nothing to release: Setenv restores the temp dir.
func (s *FixtureWindowTestSuite) AfterAll(_ *gotest.T) {}

func (s *FixtureWindowTestSuite) BeforeEach(_ *gotest.T) *windowCtx { return &windowCtx{} }

func (s *FixtureWindowTestSuite) AfterEach(_ *gotest.T, ctx *windowCtx) {
	if ctx.cleanup != nil {
		ctx.cleanup()
	}
}

const winFixturePkg = "example.com/fix"

func winFixture(id string, deps ...string) gotestgen.SharedFixtureInfo {
	return gotestgen.SharedFixtureInfo{
		Identifier:   id,
		PkgPath:      winFixturePkg,
		PkgName:      "fix",
		Dependencies: deps,
	}
}

func winKey(id string) string { return winFixturePkg + "." + id }

// windowOverlay builds a synthetic overlay: fixtures Alpha, Beta,
// Chain (depends on Alpha), and Orphan (required by nobody). Suites:
//
//	pkg/a: AlphaSuite  → Alpha        MultiSuite → Alpha, Beta
//	pkg/b: ChainSuite  → Chain (deliberately non-transitive: closure's job)
//	       TimingSuite → Beta, Exclusive
func windowOverlay() *gotestrunner.OverlayResult {
	return &gotestrunner.OverlayResult{
		SharedFixtures: []gotestgen.SharedFixtureInfo{
			winFixture("Alpha"),
			winFixture("Beta"),
			winFixture("Chain", winKey("Alpha")),
			winFixture("Orphan"),
		},
		SuitesByPkg: map[string][]string{
			"pkg/a": {"AlphaSuite", "MultiSuite"},
			"pkg/b": {"ChainSuite", "TimingSuite"},
		},
		ExclusiveSuitesByPkg: map[string]map[string]bool{
			"pkg/b": {"TimingSuite": true},
		},
		SuiteRequiredSharedFixtureKeys: map[string]map[string][]string{
			"pkg/a": {
				"TestAlphaSuite": {winKey("Alpha")},
				"TestMultiSuite": {winKey("Alpha"), winKey("Beta")},
			},
			"pkg/b": {
				"TestChainSuite":  {winKey("Chain")},
				"TestTimingSuite": {winKey("Beta")},
			},
		},
	}
}

func fixtureIdentifiers(fixtures []gotestgen.SharedFixtureInfo) []string {
	ids := make([]string, 0, len(fixtures))
	for i := range fixtures {
		ids = append(ids, fixtures[i].Identifier)
	}
	return ids
}

func (s *FixtureWindowTestSuite) TestAliveSets(t *gotest.T, _ *windowCtx) {
	overlay := windowOverlay()

	t.When("no run filter is set", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "", "")

		w.It("keeps every required fixture alive in its phase, DAG-closed", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true, winKey("Chain"): true}, win.Bulk)
			gotest.Equal(it, map[string]bool{winKey("Beta"): true}, win.Tail)
		})

		w.It("never starts the fixture no suite requires", func(it *gotest.T) {
			gotest.Equal(it, []string{"Alpha", "Beta", "Chain"}, fixtureIdentifiers(win.Fixtures), "topological overlay order, Orphan dropped")
			gotest.Equal(it, 1, win.Skipped)
		})
	})

	t.When("the run filter selects one bulk suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestAlphaSuite", "")

		w.It("keeps only that suite's fixtures", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true}, win.Bulk)
			gotest.Empty(it, win.Tail)
			gotest.Equal(it, []string{"Alpha"}, fixtureIdentifiers(win.Fixtures))
			gotest.Equal(it, 3, win.Skipped)
		})
	})

	t.When("the run filter selects only the exclusive suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestTimingSuite", "")

		w.It("plans its fixture for the tail phase only", func(it *gotest.T) {
			gotest.Empty(it, win.Bulk)
			gotest.Equal(it, map[string]bool{winKey("Beta"): true}, win.Tail)
			gotest.Equal(it, []string{"Beta"}, fixtureIdentifiers(win.Fixtures))
		})
	})

	t.When("the skip filter names every bulk suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "", "TestAlphaSuite|TestChainSuite|TestMultiSuite")

		w.It("starts no fixture for the bulk and keeps the tail's", func(it *gotest.T) {
			gotest.Empty(it, win.Bulk)
			gotest.Equal(it, map[string]bool{winKey("Beta"): true}, win.Tail)
			gotest.Equal(it, []string{"Beta"}, fixtureIdentifiers(win.Fixtures))
			gotest.Equal(it, 3, win.Skipped)
		})
	})

	t.When("the skip filter names every fuzz wrapper of a suite without a Test function", func(w *gotest.T) {
		fuzzOnly := windowOverlay()
		fuzzOnly.FuzzFuncsByPkg = map[string]map[string][]string{"pkg/b": {"ChainSuite": {"FuzzChainSuite_X"}}}
		fuzzOnly.TestlessSuitesByPkg = map[string]map[string]bool{"pkg/b": {"ChainSuite": true}}
		win := gotestrunner.ExportPlanFixtureWindows(fuzzOnly, "", "^FuzzChainSuite_X$")

		w.It("drops that suite, so its fixture does not start", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true}, win.Bulk)
		})
	})

	t.When("the skip filter names methods only", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "", "TestAlphaSuite/TestOne|TestChainSuite/TestTwo")

		w.It("drops no suite, so their fixtures start", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true, winKey("Chain"): true}, win.Bulk)
		})
	})

	t.When("the run filter carries a subtest segment", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestChainSuite/TestSomething", "")

		w.It("matches the suite on the first segment and closes over the DAG", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Chain"): true, winKey("Alpha"): true}, win.Bulk, "Alpha claimed through Chain's dependency edge")
		})
	})
}

func (s *FixtureWindowTestSuite) TestPhasePlanning(t *gotest.T, _ *windowCtx) {
	overlay := windowOverlay()

	t.When("splitting the dispatch plan into phases", func(w *gotest.T) {
		bulk, tail := gotestrunner.ExportPlanSuitePhases(overlay.SuitesByPkg, overlay.ExclusiveSuitesByPkg, nil, nil, "", "")

		w.It("routes exclusive suites to the tail and the rest to the bulk", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{"TestAlphaSuite", "TestMultiSuite"}, bulk["pkg/a"])
			gotest.Equal(it, []string{"TestChainSuite"}, bulk["pkg/b"])
			gotest.Equal(it, []string{"TestTimingSuite"}, tail["pkg/b"])
		})
	})

	t.When("a phase has no suites", func(w *gotest.T) {
		_, tail := gotestrunner.ExportPlanSuitePhases(overlay.SuitesByPkg, overlay.ExclusiveSuitesByPkg, nil, nil, "TestAlphaSuite", "")

		w.It("yields an empty alive set", func(it *gotest.T) {
			gotest.Empty(it, gotestrunner.ExportAliveFixtureKeys(tail, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures))
		})
	})
}

// groupOverlay is windowOverlay with MultiSuite in group e2e and ChainSuite
// in group load: Alpha is read by the default group and by both others,
// Beta only by e2e and the default group's exclusive suite.
func groupOverlay() *gotestrunner.OverlayResult {
	o := windowOverlay()
	o.GroupsByPkg = map[string]map[string]string{
		"pkg/a": {"MultiSuite": "e2e"},
		"pkg/b": {"ChainSuite": "load"},
	}
	return o
}

func (s *FixtureWindowTestSuite) TestGroupScopes(t *gotest.T, _ *windowCtx) {
	t.When("suites declare named groups", func(w *gotest.T) {
		scopes := gotestrunner.ExportPlanSuiteScopes(groupOverlay(), "", "")

		w.It("plans the default group first, then each named group by name", func(it *gotest.T) {
			gotest.Equal(it, []gotestrunner.ExportScope{
				{Group: "", Bulk: map[string][]string{"pkg/a": {"TestAlphaSuite"}}, Tail: map[string][]string{"pkg/b": {"TestTimingSuite"}}},
				{Group: "e2e", Bulk: map[string][]string{"pkg/a": {"TestMultiSuite"}}, Tail: map[string][]string{}},
				{Group: "load", Bulk: map[string][]string{"pkg/b": {"TestChainSuite"}}, Tail: map[string][]string{}},
			}, scopes)
		})

		w.It("starts up front only what the first scope's bulk needs", func(it *gotest.T) {
			win := gotestrunner.ExportPlanFixtureWindows(groupOverlay(), "", "")
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true}, win.Bulk)
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true, winKey("Chain"): true}, win.Tail)
		})
	})

	t.When("the selection leaves a group no suite", func(w *gotest.T) {
		w.It("plans no scope for it", func(it *gotest.T) {
			scopes := gotestrunner.ExportPlanSuiteScopes(groupOverlay(), "TestAlphaSuite|TestChainSuite", "")
			gotest.Len(it, scopes, 2)
			gotest.Equal(it, "load", scopes[1].Group)
		})
	})

	t.When("--group names a group", func(w *gotest.T) {
		w.It("plans that group alone and starts its fixtures up front", func(it *gotest.T) {
			o := groupOverlay()
			gotest.NoError(it, gotestrunner.ExportSelectGroups(o, []string{"e2e"}, nil))
			scopes := gotestrunner.ExportPlanSuiteScopes(o, "", "")
			gotest.Len(it, scopes, 1)
			gotest.Equal(it, "e2e", scopes[0].Group)
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true}, gotestrunner.ExportPlanFixtureWindows(o, "", "").Bulk)
		})
	})

	t.When("--skip-group names a group", func(w *gotest.T) {
		w.It("plans every other group", func(it *gotest.T) {
			o := groupOverlay()
			gotest.NoError(it, gotestrunner.ExportSelectGroups(o, nil, []string{"e2e"}))
			var groups []string
			for _, sc := range gotestrunner.ExportPlanSuiteScopes(o, "", "") {
				groups = append(groups, sc.Group)
			}
			gotest.Equal(it, []string{"", "load"}, groups)
		})
	})

	t.When("--group or --skip-group names a group no suite declares", func(w *gotest.T) {
		w.It("refuses it naming the known groups", func(it *gotest.T) {
			gotest.ErrorContains(it, gotestrunner.ExportSelectGroups(groupOverlay(), []string{"nope"}, nil), `--group=nope: no suite declares the group "nope"; known groups: e2e, load`)
			gotest.ErrorContains(it, gotestrunner.ExportSelectGroups(groupOverlay(), nil, []string{"E2E"}), "--skip-group=E2E")
		})
	})
}

func (s *FixtureWindowTestSuite) TestGroupRuns(t *gotest.T, _ *windowCtx) {
	targets := []gotestrunner.SuiteTarget{
		{SuiteSpec: gotestrunner.SuiteSpec{SuiteName: "TestA"}, Group: "e2e"},
		{SuiteSpec: gotestrunner.SuiteSpec{SuiteName: "TestB"}, Group: "e2e"},
		{SuiteSpec: gotestrunner.SuiteSpec{SuiteName: "TestC"}, Group: "load"},
		{SuiteSpec: gotestrunner.SuiteSpec{SuiteName: "TestD"}},
	}
	t.When("a named group's suites never ran, stopped or given up", func(w *gotest.T) {
		runs := gotestrunner.ExportGroupRuns(targets, map[string]bool{"TestA": true, "TestD": true})
		w.It("reports only the groups that ran, each with all its suites", func(it *gotest.T) {
			gotest.Len(it, runs, 1)
			gotest.Equal(it, "e2e", runs[0].Name)
			gotest.Equal(it, 2, runs[0].Suites)
		})
	})
}

// TestRealOverlayFiltering runs the planner over the real tests/sharedfixture
// packages — the same overlay the pipeline computes — so the filtering path is
// pinned against genuine discovery output, not hand-built maps.
func (s *FixtureWindowTestSuite) TestRealOverlayFiltering(t *gotest.T, ctx *windowCtx) {
	loaded, broken, err := gotestgen.LoadPackages([]string{
		"github.com/mvrahden/go-test/tests/sharedfixture/standalone/...",
		"github.com/mvrahden/go-test/tests/sharedfixture/fixturebound/...",
	}, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, nil, false, true, false)
	gotest.NoError(t, err)
	ctx.cleanup = cleanup

	t.When("the run filter matches only the fixture-free suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestPlainTestSuite", "")

		w.It("starts no shared fixture at all", func(it *gotest.T) {
			gotest.Empty(it, win.Fixtures)
			gotest.Equal(it, 3, win.Skipped, "Alpha, Beta, and Gamma all skipped")
		})
	})

	t.When("the run filter matches the Gamma suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestGammaTestSuite", "")

		w.It("keeps Gamma plus its Alpha dependency, drops Beta", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{"AlphaSharedFixture", "GammaSharedFixture"}, fixtureIdentifiers(win.Fixtures))
			gotest.Equal(it, 1, win.Skipped)
		})
	})

	t.When("no run filter is set", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "", "")

		w.It("keeps every fixture some suite requires", func(it *gotest.T) {
			gotest.Len(it, win.Fixtures, 3)
			gotest.Zero(it, win.Skipped)
		})
	})
}

// benchOverlay extends the synthetic overlay with bench suites:
//
//	pkg/a: AlphaSuite (bench, needs Alpha)   MultiSuite (bench, needs Alpha+Beta)
//	pkg/b: ChainSuite (bench, needs Chain → Alpha)
//
// Orphan stays required by nobody.
func benchOverlay() *gotestrunner.OverlayResult {
	overlay := windowOverlay()
	overlay.BenchesByPkg = map[string][]string{
		"pkg/a": {"AlphaSuite", "MultiSuite"},
		"pkg/b": {"ChainSuite"},
	}
	return overlay
}

func (s *FixtureWindowTestSuite) TestBenchWindowPlanning(t *gotest.T, _ *windowCtx) {
	overlay := benchOverlay()

	t.When("no filters are set", func(w *gotest.T) {
		win := gotestrunner.ExportPlanBenchFixtureWindows(overlay, "", "", "")

		w.It("starts only the first slot's fixtures up-front and defers the rest", func(it *gotest.T) {
			// Dispatch order: pkg/a AlphaSuite, pkg/a MultiSuite, pkg/b ChainSuite.
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true}, win.Bulk)
			deferred := map[string]bool{}
			for i := range win.Fixtures {
				deferred[win.Fixtures[i].Identifier] = win.Fixtures[i].Deferred
			}
			gotest.Equal(it, map[string]bool{"Alpha": false, "Beta": true, "Chain": true}, deferred)
		})

		w.It("never starts the fixture no bench suite requires", func(it *gotest.T) {
			gotest.Equal(it, 1, win.Skipped, "Orphan is not in the plan")
		})
	})

	t.When("-bench selects only the chain suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanBenchFixtureWindows(overlay, "", "BenchmarkChainSuite", "")

		w.It("keeps its DAG-closed needs and nothing else, all up-front", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Chain"): true, winKey("Alpha"): true}, win.Bulk,
				"the one slot is the first slot: its closure starts with compile")
			gotest.ElementsMatch(it, []string{"Alpha", "Chain"}, fixtureIdentifiers(win.Fixtures))
			gotest.Equal(it, 2, win.Skipped)
		})
	})

	t.When("-skip names a bench suite's wrapper", func(w *gotest.T) {
		win := gotestrunner.ExportPlanBenchFixtureWindows(overlay, "", "", "^BenchmarkAlphaSuite$|^BenchmarkChainSuite$")

		w.It("plans no fixture for it", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true}, win.Bulk, "MultiSuite's needs only")
			gotest.Equal(it, 2, win.Skipped)
		})
	})

	t.When("-run matches Benchmark names, not test names", func(w *gotest.T) {
		win := gotestrunner.ExportPlanBenchFixtureWindows(overlay, "^BenchmarkMultiSuite$", "", "")

		w.It("plans for the bench suites the filter selects", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true}, win.Bulk)
			gotest.Equal(it, 2, win.Skipped)
		})
	})
}

func (s *FixtureWindowTestSuite) TestBenchSlotPlan(t *gotest.T, _ *windowCtx) {
	overlay := benchOverlay()
	targets := []gotestrunner.SuiteTarget{
		{SuiteSpec: gotestrunner.SuiteSpec{Package: "pkg/a", SuiteName: "AlphaSuite"}},
		{SuiteSpec: gotestrunner.SuiteSpec{Package: "pkg/a", SuiteName: "MultiSuite"}},
		{SuiteSpec: gotestrunner.SuiteSpec{Package: "pkg/b", SuiteName: "ChainSuite"}},
	}

	t.When("computing per-slot windows", func(w *gotest.T) {
		needs, laterNeeds := gotestrunner.ExportBenchSlotPlan(targets, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures)

		w.It("resolves each slot's DAG-closed needs", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true}, needs[0])
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true}, needs[1])
			gotest.Equal(it, map[string]bool{winKey("Chain"): true, winKey("Alpha"): true}, needs[2])
		})

		w.It("keeps a fixture resident until its last slot, then releases it", func(it *gotest.T) {
			// Alpha is needed by every slot: it must survive slot 1 even
			// though slot 1 also needs Beta — resident through the whole run.
			gotest.True(it, laterNeeds[1][winKey("Alpha")])
			gotest.True(it, laterNeeds[2][winKey("Alpha")], "Chain's closure keeps Alpha alive through the last slot")
			gotest.False(it, laterNeeds[2][winKey("Beta")], "Beta's window closes after its only slot")
			gotest.Empty(it, laterNeeds[3], "after the final slot nothing is needed")
		})
	})
}

// dependencyOrderViolations lists every fixture that comes before one of its
// dependencies. The setup process starts and releases fixtures in this order,
// so a dependent placed first waits for a dependency nothing has started.
func dependencyOrderViolations(fixtures []gotestgen.SharedFixtureInfo) []string {
	pos := map[string]int{}
	for i := range fixtures {
		pos[fixtures[i].PkgPath+"."+fixtures[i].Identifier] = i
	}
	var out []string
	for i := range fixtures {
		for _, dep := range fixtures[i].Dependencies {
			if j, ok := pos[dep]; ok && j > i {
				out = append(out, fixtures[i].Identifier+" before "+dep)
			}
		}
	}
	return out
}

// depOverlay lists App (depends on DB) before DB, in the name order discovery
// returns them. Suites:
//
//	pkg/a: BulkSuite → nothing    SharedSuite → DB
//	       TailSuite → App, Exclusive
//	       BenchFree (bench, first slot) → nothing
//	       BenchApp  (bench, later slot) → App
func depOverlay() *gotestrunner.OverlayResult {
	return &gotestrunner.OverlayResult{
		SharedFixtures: []gotestgen.SharedFixtureInfo{
			winFixture("App", winKey("DB")),
			winFixture("DB"),
		},
		SuitesByPkg: map[string][]string{
			"pkg/a": {"BulkSuite", "SharedSuite", "TailSuite"},
		},
		ExclusiveSuitesByPkg: map[string]map[string]bool{
			"pkg/a": {"TailSuite": true},
		},
		BenchesByPkg: map[string][]string{
			"pkg/a": {"BenchApp", "ABenchFree"},
		},
		SuiteRequiredSharedFixtureKeys: map[string]map[string][]string{
			"pkg/a": {
				"TestSharedSuite": {winKey("DB")},
				"TestTailSuite":   {winKey("App"), winKey("DB")},
				"TestBenchApp":    {winKey("App"), winKey("DB")},
			},
		},
	}
}

func (s *FixtureWindowTestSuite) TestDependencyOrder(t *gotest.T, ctx *windowCtx) {
	loaded, broken, err := gotestgen.LoadPackages([]string{
		"github.com/mvrahden/go-test/tests/conformance/testdata/fixataildep",
	}, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)
	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, nil, false, true, false)
	gotest.NoError(t, err)
	ctx.cleanup = cleanup

	t.When("a tail-only fixture's tail-only dependency sorts after it by name", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "", "")

		w.It("defers both and lists the dependency first", func(it *gotest.T) {
			gotest.Empty(it, win.Bulk)
			gotest.Len(it, win.Tail, 2)
			gotest.Equal(it, []string{"DBSharedFixture", "AppSharedFixture"}, fixtureIdentifiers(win.Fixtures))
			for i := range win.Fixtures {
				gotest.True(it, win.Fixtures[i].Deferred, win.Fixtures[i].Identifier)
			}
		})
	})

	t.When("a dependency is shared between the bulk and the tail", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(depOverlay(), "TestSharedSuite|TestTailSuite", "")

		w.It("keeps the dependency up across the barrier and lists it first", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("DB"): true}, win.Bulk)
			gotest.Equal(it, map[string]bool{winKey("App"): true, winKey("DB"): true}, win.Tail)
			gotest.Empty(it, dependencyOrderViolations(win.Fixtures))
		})
	})

	t.When("a later bench slot needs a fixture and its dependency", func(w *gotest.T) {
		win := gotestrunner.ExportPlanBenchFixtureWindows(depOverlay(), "", "", "")

		w.It("defers both and lists the dependency first", func(it *gotest.T) {
			gotest.Empty(it, win.Bulk)
			gotest.Equal(it, []string{"DB", "App"}, fixtureIdentifiers(win.Fixtures))
		})
	})
}
