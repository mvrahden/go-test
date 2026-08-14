package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureWindowTestSuite pins the alive-set math behind fixture window
// scheduling: which shared fixtures a run keeps resident, per phase, given
// the suites it will actually dispatch.
type FixtureWindowTestSuite struct {
	cleanup func()
}

func (s *FixtureWindowTestSuite) AfterEach(_ *gotest.T) {
	if s.cleanup != nil {
		s.cleanup()
		s.cleanup = nil
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

func (s *FixtureWindowTestSuite) TestAliveSets(t *gotest.T) {
	overlay := windowOverlay()

	t.When("no run filter is set", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "")

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
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestAlphaSuite")

		w.It("keeps only that suite's fixtures", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true}, win.Bulk)
			gotest.Empty(it, win.Tail)
			gotest.Equal(it, []string{"Alpha"}, fixtureIdentifiers(win.Fixtures))
			gotest.Equal(it, 3, win.Skipped)
		})
	})

	t.When("the run filter selects only the exclusive suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestTimingSuite")

		w.It("plans its fixture for the tail phase only", func(it *gotest.T) {
			gotest.Empty(it, win.Bulk)
			gotest.Equal(it, map[string]bool{winKey("Beta"): true}, win.Tail)
			gotest.Equal(it, []string{"Beta"}, fixtureIdentifiers(win.Fixtures))
		})
	})

	t.When("the run filter carries a subtest segment", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestChainSuite/TestSomething")

		w.It("matches the suite on the first segment and closes over the DAG", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Chain"): true, winKey("Alpha"): true}, win.Bulk, "Alpha claimed through Chain's dependency edge")
		})
	})
}

func (s *FixtureWindowTestSuite) TestPhasePlanning(t *gotest.T) {
	overlay := windowOverlay()

	t.When("splitting the dispatch plan into phases", func(w *gotest.T) {
		bulk, tail := gotestrunner.ExportPlanSuitePhases(overlay.SuitesByPkg, overlay.ExclusiveSuitesByPkg, "")

		w.It("routes exclusive suites to the tail and the rest to the bulk", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{"TestAlphaSuite", "TestMultiSuite"}, bulk["pkg/a"])
			gotest.Equal(it, []string{"TestChainSuite"}, bulk["pkg/b"])
			gotest.Equal(it, []string{"TestTimingSuite"}, tail["pkg/b"])
		})
	})

	t.When("a phase has no suites", func(w *gotest.T) {
		_, tail := gotestrunner.ExportPlanSuitePhases(overlay.SuitesByPkg, overlay.ExclusiveSuitesByPkg, "TestAlphaSuite")

		w.It("yields an empty alive set", func(it *gotest.T) {
			gotest.Empty(it, gotestrunner.ExportAliveFixtureKeys(tail, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures))
		})
	})
}

// TestRealOverlayFiltering runs the planner over the real tests/sharedfixture
// packages — the same overlay the pipeline computes — so the filtering path is
// pinned against genuine discovery output, not hand-built maps.
func (s *FixtureWindowTestSuite) TestRealOverlayFiltering(t *gotest.T) {
	loaded, broken, err := gotestgen.LoadPackages([]string{
		"github.com/mvrahden/go-test/tests/sharedfixture/standalone/...",
		"github.com/mvrahden/go-test/tests/sharedfixture/fixturebound/...",
	}, nil)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, nil, false, true)
	gotest.NoError(t, err)
	s.cleanup = cleanup

	t.When("the run filter matches only the fixture-free suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestPlainTestSuite")

		w.It("starts no shared fixture at all", func(it *gotest.T) {
			gotest.Empty(it, win.Fixtures)
			gotest.Equal(it, 3, win.Skipped, "Alpha, Beta, and Gamma all skipped")
		})
	})

	t.When("the run filter matches the Gamma suite", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "TestGammaTestSuite")

		w.It("keeps Gamma plus its Alpha dependency, drops Beta", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{"AlphaSharedFixture", "GammaSharedFixture"}, fixtureIdentifiers(win.Fixtures))
			gotest.Equal(it, 1, win.Skipped)
		})
	})

	t.When("no run filter is set", func(w *gotest.T) {
		win := gotestrunner.ExportPlanFixtureWindows(overlay, "")

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

func (s *FixtureWindowTestSuite) TestBenchWindowPlanning(t *gotest.T) {
	overlay := benchOverlay()

	t.When("no filters are set", func(w *gotest.T) {
		win := gotestrunner.ExportPlanBenchFixtureWindows(overlay, "", "")

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
		win := gotestrunner.ExportPlanBenchFixtureWindows(overlay, "", "BenchmarkChainSuite")

		w.It("keeps its DAG-closed needs and nothing else, all up-front", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Chain"): true, winKey("Alpha"): true}, win.Bulk,
				"the one slot is the first slot: its closure starts with compile")
			gotest.ElementsMatch(it, []string{"Alpha", "Chain"}, fixtureIdentifiers(win.Fixtures))
			gotest.Equal(it, 2, win.Skipped)
		})
	})

	t.When("-run matches Benchmark names, not test names", func(w *gotest.T) {
		win := gotestrunner.ExportPlanBenchFixtureWindows(overlay, "^BenchmarkMultiSuite$", "")

		w.It("plans for the bench suites the filter selects", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{winKey("Alpha"): true, winKey("Beta"): true}, win.Bulk)
			gotest.Equal(it, 2, win.Skipped)
		})
	})
}

func (s *FixtureWindowTestSuite) TestBenchSlotPlan(t *gotest.T) {
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
