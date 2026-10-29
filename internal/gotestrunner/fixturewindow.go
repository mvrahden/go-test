package gotestrunner

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// Fixture window scheduling.
//
// Doctrine: fixtures are never speculative — a shared fixture is resident
// exactly while a scheduled suite needs it. A run is a sequence of scopes
// (planSuiteScopes), each the parallel bulk of a group, then its exclusive
// serial tail (SuiteTarget.Exclusive). The first scope's bulk streams with
// the compile; Alive(phase) is the DAG-closure of the union of the phase
// targets' required shared fixture keys. Fixtures in no alive set never
// start — and can therefore never fail the run. Alive of the first bulk
// starts up-front, concurrent with compile; every later phase starts its
// own just before it; run-end teardown owns everything still resident.

// fixtureWindows is one run's shared fixture residency plan, computed from
// the overlay and the user's -run filter before any fixture starts.
type fixtureWindows struct {
	Bulk map[string]bool // Alive(first bulk): state keys the streamed bulk needs
	Tail map[string]bool // Alive(later phases): state keys every phase after it needs
	// Fixtures is Alive(bulk) ∪ Alive(tail), dependencies first — the only
	// fixtures the setup subprocess is generated with, in the order it starts
	// a command's fixtures and the reverse of the order it releases them.
	Fixtures []gotestgen.SharedFixtureInfo
	// Skipped counts overlay fixtures no scheduled suite requires.
	Skipped int
}

// planFixtureWindows computes the residency plan for a run. The suite
// selection mirrors BuildSuiteTargets minus compile results, which do not
// exist yet: fixtures must not wait for the compiler.
func planFixtureWindows(overlay *OverlayResult, userRunFilter, userSkipFilter string) fixtureWindows {
	scopes := planSuiteScopes(overlay, userRunFilter, userSkipFilter)
	var first map[string][]string
	later := map[string][]string{}
	for i, sc := range scopes {
		if i == 0 {
			first = sc.Bulk
		} else {
			mergePhase(later, sc.Bulk)
		}
		mergePhase(later, sc.Tail)
	}
	w := fixtureWindows{
		Bulk: aliveFixtureKeys(first, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures),
		Tail: aliveFixtureKeys(later, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures),
	}
	for i := range overlay.SharedFixtures {
		key := sharedFixtureKey(&overlay.SharedFixtures[i])
		switch {
		case w.Bulk[key]:
			w.Fixtures = append(w.Fixtures, overlay.SharedFixtures[i])
		case w.Tail[key]:
			// Late fixtures ride along deferred: compiled into the setup
			// program, started by StartKeys just before their phase.
			sf := overlay.SharedFixtures[i]
			sf.Deferred = true
			w.Fixtures = append(w.Fixtures, sf)
		default:
			w.Skipped++
		}
	}
	w.Fixtures = dependenciesFirst(w.Fixtures)
	return w
}

func mergePhase(into, phase map[string][]string) {
	for pkg, suites := range phase {
		into[pkg] = append(into[pkg], suites...)
	}
}

// dependenciesFirst orders fixtures so each comes after its dependencies,
// keeping the given order otherwise, so the generated setup program lists
// them the same way on every run. The setup process orders starts and
// releases by the dependencies themselves, whatever the list order.
func dependenciesFirst(fixtures []gotestgen.SharedFixtureInfo) []gotestgen.SharedFixtureInfo {
	byKey := make(map[string]int, len(fixtures))
	for i := range fixtures {
		byKey[sharedFixtureKey(&fixtures[i])] = i
	}
	out := make([]gotestgen.SharedFixtureInfo, 0, len(fixtures))
	placed := make([]bool, len(fixtures))
	var place func(i int)
	place = func(i int) {
		if placed[i] {
			return
		}
		placed[i] = true
		for _, dep := range fixtures[i].Dependencies {
			if j, ok := byKey[dep]; ok {
				place(j)
			}
		}
		out = append(out, fixtures[i])
	}
	for i := range fixtures {
		place(i)
	}
	return out
}

// diffKeys returns set ∖ minus, sorted for deterministic commands and
// messages. Execution order is the subprocess's job — it applies its
// generated (reverse-)topological order regardless of the order sent.
func diffKeys(set, minus map[string]bool) []string {
	var out []string
	for k := range set {
		if !minus[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// reportSkipped emits the one debug line for fixtures the plan never starts.
func (w *fixtureWindows) reportSkipped() {
	if w.Skipped > 0 {
		fmt.Fprintf(os.Stderr, "gotest: %d shared fixture(s) not started (no scheduled suite requires them)\n", w.Skipped)
	}
}

// suiteScope is one group's share of a run: its parallel bulk, then its
// exclusive tail, as planSuitePhases splits them. Scopes never overlap.
type suiteScope struct {
	Group      string // "" for the default group
	Bulk, Tail map[string][]string
}

// planSuiteScopes splits the suites the run will dispatch into scopes: the
// default group first, then each named group by name. A group the
// selection leaves no suite in has no scope.
func planSuiteScopes(overlay *OverlayResult, userRunFilter, userSkipFilter string) []suiteScope {
	byGroup := map[string]map[string][]string{}
	for pkg, suites := range overlay.SuitesByPkg {
		for _, suiteName := range suites {
			group := overlay.GroupsByPkg[pkg][suiteName]
			if !overlay.groups.selects(group) {
				continue
			}
			if byGroup[group] == nil {
				byGroup[group] = map[string][]string{}
			}
			byGroup[group][pkg] = append(byGroup[group][pkg], suiteName)
		}
	}
	scopes := make([]suiteScope, 0, len(byGroup))
	for _, group := range slices.Sorted(maps.Keys(byGroup)) {
		bulk, tail := planSuitePhases(byGroup[group], overlay.ExclusiveSuitesByPkg, overlay.FuzzFuncsByPkg, overlay.TestlessSuitesByPkg, userRunFilter, userSkipFilter)
		if len(bulk) == 0 && len(tail) == 0 {
			continue
		}
		scopes = append(scopes, suiteScope{Group: group, Bulk: bulk, Tail: tail})
	}
	return scopes
}

// planSuitePhases splits the suites the run will dispatch into the two phases:
// those -run selects and -skip does not drop whole.
// Values are test function names ("Test" + suite name) — the keying
// SuiteRequiredSharedFixtureKeys uses.
func planSuitePhases(suitesByPkg map[string][]string, exclusiveByPkg map[string]map[string]bool, fuzzFuncsByPkg map[string]map[string][]string, testlessByPkg map[string]map[string]bool, userRunFilter, userSkipFilter string) (bulk, tail map[string][]string) {
	bulk, tail = map[string][]string{}, map[string][]string{}
	for pkg, suites := range suitesByPkg {
		for _, suiteName := range suites {
			testFuncName := "Test" + suiteName
			funcs := suiteFuncs(suiteName, testlessByPkg[pkg][suiteName], fuzzFuncsByPkg[pkg][suiteName])
			if !selectsSuite(userRunFilter, funcs) || skipsSuite(userSkipFilter, funcs) {
				continue
			}
			phase := bulk
			if exclusiveByPkg[pkg][suiteName] {
				phase = tail
			}
			phase[pkg] = append(phase[pkg], testFuncName)
		}
	}
	return bulk, tail
}

// aliveFixtureKeys computes Alive(phase): the DAG-closure of the union of the
// phase suites' required shared fixture keys. Per-suite keys are already
// transitive; closing over fixture dependencies here keeps the invariant
// local — an alive fixture's dependencies are alive, whatever produced the
// per-suite sets.
func aliveFixtureKeys(phaseSuites map[string][]string, reqKeys map[string]map[string][]string, fixtures []gotestgen.SharedFixtureInfo) map[string]bool {
	deps := make(map[string][]string, len(fixtures))
	for i := range fixtures {
		deps[sharedFixtureKey(&fixtures[i])] = fixtures[i].Dependencies
	}
	alive := map[string]bool{}
	var claim func(key string)
	claim = func(key string) {
		if alive[key] {
			return
		}
		alive[key] = true
		for _, dep := range deps[key] {
			claim(dep)
		}
	}
	for pkg, suites := range phaseSuites {
		for _, suite := range suites {
			for _, key := range reqKeys[pkg][suite] {
				claim(key)
			}
		}
	}
	return alive
}

// sharedFixtureKey returns the fixture's state key — the identity every map
// in the shared fixture protocol is keyed by.
func sharedFixtureKey(sf *gotestgen.SharedFixtureInfo) string {
	return sf.PkgPath + "." + sf.Identifier
}

// Bench windows: bench dispatch is strictly serial, so every slot is its own
// phase. A fixture is resident from the first slot that needs it until the
// last — opened by StartKeys just before the first, released by TeardownKeys
// right after the last, never restarted in between.

// planBenchFixtureWindows computes the residency plan for a bench run. The
// selection mirrors BuildBenchTargets minus compile results: -run, -bench and
// -skip are matched against Benchmark<Suite>, not the test function names the
// default plan uses. Bulk is the first planned slot's needs — started
// up-front, concurrent with compile; every other fixture rides deferred and
// opens with its slot.
func planBenchFixtureWindows(overlay *OverlayResult, userRunFilter, userBenchFilter, userSkipFilter string) fixtureWindows {
	type slot struct {
		pkg   string
		suite string
	}
	var slots []slot
	for pkg, suites := range overlay.BenchesByPkg {
		for _, suiteName := range suites {
			if !selectsBench(userRunFilter, userBenchFilter, userSkipFilter, "Benchmark"+suiteName) {
				continue
			}
			slots = append(slots, slot{pkg: pkg, suite: suiteName})
		}
	}
	// Dispatch order: the same deterministic (Package, SuiteName) order the
	// serial dispatcher applies.
	sort.Slice(slots, func(a, b int) bool {
		if slots[a].pkg != slots[b].pkg {
			return slots[a].pkg < slots[b].pkg
		}
		return slots[a].suite < slots[b].suite
	})

	all := map[string][]string{}
	for _, sl := range slots {
		all[sl.pkg] = append(all[sl.pkg], "Test"+sl.suite)
	}
	w := fixtureWindows{
		Bulk: map[string]bool{},
		Tail: map[string]bool{},
	}
	if len(slots) > 0 {
		first := map[string][]string{slots[0].pkg: {"Test" + slots[0].suite}}
		w.Bulk = aliveFixtureKeys(first, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures)
	}
	alive := aliveFixtureKeys(all, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures)
	for i := range overlay.SharedFixtures {
		key := sharedFixtureKey(&overlay.SharedFixtures[i])
		switch {
		case w.Bulk[key]:
			w.Fixtures = append(w.Fixtures, overlay.SharedFixtures[i])
		case alive[key]:
			sf := overlay.SharedFixtures[i]
			sf.Deferred = true
			w.Fixtures = append(w.Fixtures, sf)
		default:
			w.Skipped++
		}
	}
	w.Fixtures = dependenciesFirst(w.Fixtures)
	return w
}

// benchSlotPlan computes each slot's needed keys and the union of every later
// slot's needs, for bench targets already in dispatch order. laterNeeds has
// len(targets)+1 entries; the final one is empty — after the last slot,
// nothing is needed.
func benchSlotPlan(targets []SuiteTarget, reqKeys map[string]map[string][]string, fixtures []gotestgen.SharedFixtureInfo) (needs, laterNeeds []map[string]bool) {
	needs = make([]map[string]bool, len(targets))
	for i := range targets {
		// Bench targets carry the bare suite identifier; required keys are
		// keyed by test function name.
		phase := map[string][]string{targets[i].Package: {"Test" + targets[i].SuiteName}}
		needs[i] = aliveFixtureKeys(phase, reqKeys, fixtures)
	}
	laterNeeds = make([]map[string]bool, len(targets)+1)
	laterNeeds[len(targets)] = map[string]bool{}
	for i := len(targets) - 1; i >= 0; i-- {
		later := make(map[string]bool, len(laterNeeds[i+1])+len(needs[i]))
		for k := range laterNeeds[i+1] {
			later[k] = true
		}
		for k := range needs[i] {
			later[k] = true
		}
		laterNeeds[i] = later
	}
	return needs, laterNeeds
}
