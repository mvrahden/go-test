package gotestrunner

import (
	"fmt"
	"os"
	"sort"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// Fixture window scheduling.
//
// Doctrine: fixtures are never speculative — a shared fixture is resident
// exactly while a scheduled suite needs it. A run has two dispatch phases:
// the parallel bulk, then the exclusive serial tail (SuiteTarget.Exclusive).
// Alive(phase) is the DAG-closure of the union of the phase targets' required
// shared fixture keys. Fixtures in neither alive set never start — and can
// therefore never fail the run. Alive(bulk) starts up-front, concurrent with
// compile; run-end teardown owns everything still resident.

// fixtureWindows is one run's shared fixture residency plan, computed from
// the overlay and the user's -run filter before any fixture starts.
type fixtureWindows struct {
	Bulk map[string]bool // Alive(bulk): state keys the parallel bulk needs
	Tail map[string]bool // Alive(tail): state keys the exclusive tail needs
	// Fixtures is Alive(bulk) ∪ Alive(tail) in the overlay's topological
	// order — the only fixtures the setup subprocess is generated with.
	Fixtures []gotestgen.SharedFixtureInfo
	// Skipped counts overlay fixtures no scheduled suite requires.
	Skipped int
}

// planFixtureWindows computes the residency plan for a run. The suite
// selection mirrors BuildSuiteTargets minus compile results, which do not
// exist yet: fixtures must not wait for the compiler.
func planFixtureWindows(overlay *OverlayResult, userRunFilter string) fixtureWindows {
	bulk, tail := planSuitePhases(overlay.SuitesByPkg, overlay.ExclusiveSuitesByPkg, userRunFilter)
	w := fixtureWindows{
		Bulk: aliveFixtureKeys(bulk, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures),
		Tail: aliveFixtureKeys(tail, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures),
	}
	for i := range overlay.SharedFixtures {
		key := sharedFixtureKey(&overlay.SharedFixtures[i])
		switch {
		case w.Bulk[key]:
			w.Fixtures = append(w.Fixtures, overlay.SharedFixtures[i])
		case w.Tail[key]:
			// Tail-only fixtures ride along deferred: compiled into the setup
			// program, started by StartKeys at the bulk→tail barrier.
			sf := overlay.SharedFixtures[i]
			sf.Deferred = true
			w.Fixtures = append(w.Fixtures, sf)
		default:
			w.Skipped++
		}
	}
	return w
}

// aliveFromTargets computes the alive set for one phase's actual targets —
// the plan narrowed by compile results. Targets carry test function names in
// SuiteName, matching the required-keys maps.
func aliveFromTargets(targets []SuiteTarget, reqKeys map[string]map[string][]string, fixtures []gotestgen.SharedFixtureInfo) map[string]bool {
	phase := map[string][]string{}
	for i := range targets {
		phase[targets[i].Package] = append(phase[targets[i].Package], targets[i].SuiteName)
	}
	return aliveFixtureKeys(phase, reqKeys, fixtures)
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

// planSuitePhases splits the suites the run will dispatch into the two phases.
// Values are test function names ("Test" + suite name) — the keying
// SuiteRequiredSharedFixtureKeys uses.
func planSuitePhases(suitesByPkg map[string][]string, exclusiveByPkg map[string]map[string]bool, userRunFilter string) (bulk, tail map[string][]string) {
	bulk, tail = map[string][]string{}, map[string][]string{}
	for pkg, suites := range suitesByPkg {
		for _, suiteName := range suites {
			testFuncName := "Test" + suiteName
			if userRunFilter != "" && !matchesSuiteFunc(userRunFilter, testFuncName) {
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
