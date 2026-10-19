package canary_test //nolint:fail-guard

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A shared fixture started for the exclusive tail is released under the
// teardown budget its declared 2s Timeout gives: its AfterAll, which never
// returns, is force-killed once 32s have passed, not the default's 2m30s.
func (s *CanaryTimingTestSuite) TestALateFixtureIsReleasedUnderItsDeclaredBudget(t *gotest.T) {
	dir := s.warm(t, "latebudget")
	began := time.Now()
	r := s.checkFixture(t, "latebudget", dir)
	if !strings.Contains(r.stderr, "force-killed after 32s;") {
		t.Errorf("latebudget: the late fixture was not released under its 32s teardown budget:\n%s", r.stderr)
	}
	if took := time.Since(began); took > 90*time.Second {
		t.Errorf("latebudget: the run took %s; the release outlived the fixture's 32s teardown budget", took)
	}
}
