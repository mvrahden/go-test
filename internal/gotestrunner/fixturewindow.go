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

// planFixtureWindows computes the residency plan for a run from the same
// selection the targets are built from, before anything compiles: fixtures
// must not wait for the compiler.
func planFixtureWindows(plan *RunPlan, fixtures []gotestgen.SharedFixtureInfo, sel selection) fixtureWindows { //nolint:gocritic // hugeParam: once per run
	scopes := planSuiteScopes(plan, sel)
	var first, later []*PlannedSuite
	for i, sc := range scopes {
		if i == 0 {
			first = flatten(sc.Bulk)
		} else {
			later = append(later, flatten(sc.Bulk)...)
		}
		later = append(later, flatten(sc.Tail)...)
	}
	w := fixtureWindows{
		Bulk: aliveFixtureKeys(first, fixtures),
		Tail: aliveFixtureKeys(later, fixtures),
	}
	for i := range fixtures {
		key := sharedFixtureKey(&fixtures[i])
		switch {
		case w.Bulk[key]:
			w.Fixtures = append(w.Fixtures, fixtures[i])
		case w.Tail[key]:
			// Late fixtures ride along deferred: compiled into the setup
			// program, started by StartKeys just before their phase.
			sf := fixtures[i]
			sf.Deferred = true
			w.Fixtures = append(w.Fixtures, sf)
		default:
			w.Skipped++
		}
	}
	w.Fixtures = dependenciesFirst(w.Fixtures)
	return w
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
// exclusive tail, each by package. Scopes never overlap.
type suiteScope struct {
	Group      string // "" for the default group
	Bulk, Tail map[string][]*PlannedSuite
}

// suites lists the scope's bulk, then its tail, packages sorted.
func (sc *suiteScope) suites() []*PlannedSuite {
	return append(flatten(sc.Bulk), flatten(sc.Tail)...)
}

// planSuiteScopes splits the suites the run will dispatch into scopes: the
// default group first, then each named group by name. A group the
// selection leaves no suite in has no scope.
func planSuiteScopes(plan *RunPlan, sel selection) []suiteScope { //nolint:gocritic // hugeParam: once per run
	byGroup := map[string]*suiteScope{}
	for _, s := range plan.testRun(sel) {
		sc := byGroup[s.Group]
		if sc == nil {
			sc = &suiteScope{Group: s.Group, Bulk: map[string][]*PlannedSuite{}, Tail: map[string][]*PlannedSuite{}}
			byGroup[s.Group] = sc
		}
		phase := sc.Bulk
		if s.Exclusive {
			phase = sc.Tail
		}
		phase[s.Package] = append(phase[s.Package], s)
	}
	scopes := make([]suiteScope, 0, len(byGroup))
	for _, group := range slices.Sorted(maps.Keys(byGroup)) {
		scopes = append(scopes, *byGroup[group])
	}
	return scopes
}

// aliveFixtureKeys computes Alive(phase): the DAG-closure of the union of the
// phase suites' required shared fixture keys. Per-suite keys are already
// transitive; closing over fixture dependencies here keeps the invariant
// local — an alive fixture's dependencies are alive, whatever produced the
// per-suite sets.
func aliveFixtureKeys(suites []*PlannedSuite, fixtures []gotestgen.SharedFixtureInfo) map[string]bool {
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
	for _, s := range suites {
		if s == nil {
			continue
		}
		for _, key := range s.SharedKeys {
			claim(key)
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

// planBenchFixtureWindows computes the residency plan for a bench run from
// the bench run's selection: -run, -bench and -skip are matched against
// Benchmark<Suite>, not the test function names the default plan uses. Bulk
// is the first planned slot's needs — started up-front, concurrent with
// compile; every other fixture rides deferred and opens with its slot.
func planBenchFixtureWindows(plan *RunPlan, fixtures []gotestgen.SharedFixtureInfo, sel selection) fixtureWindows { //nolint:gocritic // hugeParam: once per run
	// Dispatch order: the deterministic (Package, Name) order the serial
	// dispatcher applies.
	slots := plan.benchRun(sel)
	w := fixtureWindows{
		Bulk: map[string]bool{},
		Tail: map[string]bool{},
	}
	if len(slots) > 0 {
		w.Bulk = aliveFixtureKeys(slots[:1], fixtures)
	}
	alive := aliveFixtureKeys(slots, fixtures)
	for i := range fixtures {
		key := sharedFixtureKey(&fixtures[i])
		switch {
		case w.Bulk[key]:
			w.Fixtures = append(w.Fixtures, fixtures[i])
		case alive[key]:
			sf := fixtures[i]
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
// slot's needs, for slots already in dispatch order. laterNeeds has
// len(slots)+1 entries; the final one is empty — after the last slot,
// nothing is needed.
func benchSlotPlan(slots []*PlannedSuite, fixtures []gotestgen.SharedFixtureInfo) (needs, laterNeeds []map[string]bool) {
	needs = make([]map[string]bool, len(slots))
	for i := range slots {
		needs[i] = aliveFixtureKeys(slots[i:i+1], fixtures)
	}
	laterNeeds = make([]map[string]bool, len(slots)+1)
	laterNeeds[len(slots)] = map[string]bool{}
	for i := len(slots) - 1; i >= 0; i-- {
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
