package gotestrunner

import (
	"context"
	"fmt"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol"
)

// FuzzFixtures holds the shared fixtures a fuzz session's targets bind, for
// the whole session: started before any target runs and torn down after the
// last. Each target's processes, fuzz workers included, only hydrate them.
type FuzzFixtures struct {
	proc    *SharedFixtureProcess
	reqKeys map[string]map[string][]string
}

// StartFuzzFixtures starts the shared fixtures the suites of targets bind and
// waits for their setup; setupTimeout is as for a run, 0 the default. It
// returns nil when no target binds one. A fixture that fails to come up gives
// up the targets that read it, not the session: see Env.
func StartFuzzFixtures(ctx context.Context, overlay *OverlayResult, targets []FuzzTarget, setupTimeout time.Duration) (*FuzzFixtures, error) {
	suites := map[string][]string{}
	for i := range targets {
		suites[targets[i].Package] = append(suites[targets[i].Package], "Test"+targets[i].Suite)
	}
	alive := aliveFixtureKeys(suites, overlay.SuiteRequiredSharedFixtureKeys, overlay.SharedFixtures)
	if len(alive) == 0 {
		return nil, nil
	}
	var fixtures []gotestgen.SharedFixtureInfo
	for i := range overlay.SharedFixtures {
		if alive[sharedFixtureKey(&overlay.SharedFixtures[i])] {
			fixtures = append(fixtures, overlay.SharedFixtures[i])
		}
	}
	setupTimeout = resolveSetupTimeout(setupTimeout)
	proc, err := StartSharedFixtures(ctx, overlay.WorkDir, fixtures, setupTimeout)
	if err != nil {
		return nil, fmt.Errorf("shared fixture setup failed: %w", err)
	}
	// What failed is on stderr already, from the fixture process; the targets
	// that read it say so in Env.
	_, _ = awaitSharedSetup(ctx, proc, setupTimeout)
	return &FuzzFixtures{proc: proc, reqKeys: overlay.SuiteRequiredSharedFixtureKeys}, nil
}

// Env returns the environment entries t's processes read their shared
// fixtures through, or, when a fixture t binds did not come up, why t never
// ran. A nil FuzzFixtures has nothing to hand out.
func (f *FuzzFixtures) Env(t FuzzTarget) (env []string, neverRanReason string) { //nolint:gocritic // hugeParam: FuzzTarget is passed by value throughout
	if f == nil {
		return nil, ""
	}
	keys := f.reqKeys[t.Package]["Test"+t.Suite]
	if len(keys) == 0 {
		return nil, ""
	}
	if missing := missingFixture(f.proc, keys); missing != "" {
		return nil, neverRan(t.Func, missing)
	}
	path, err := f.proc.WriteSuiteStateFile(t.Package, t.Func, keys)
	if err != nil {
		return nil, fmt.Sprintf("%s never ran: shared fixture state: %s", t.Func, err)
	}
	return []string{protocol.EnvSharedStateFile + "=" + path}, ""
}

// Teardown releases the fixtures and reports what failed to release.
func (f *FuzzFixtures) Teardown() error {
	if f == nil {
		return nil
	}
	return f.proc.Teardown()
}
