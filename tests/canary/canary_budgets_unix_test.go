//go:build unix

package canary_test //nolint:fail-guard

import (
	"slices"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// budgetStopped runs the stopbudget fixture with env, interrupts it once its
// method holds and returns how long the run took to end after that, its exit
// code and the markers. A run still going 90s after the stop is killed.
func (s *CanaryBudgetsTestSuite) budgetStopped(t *gotest.T, env ...string) (took time.Duration, code int, out string, markers []string) {
	r := s.startRun(t, t.TempDir(), env, "-json", "./testdata/stopbudget/")
	if !r.await(90*time.Second, "running") {
		_, _ = r.end(0)
		fatalf(t, "stopbudget: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	began := time.Now()
	if err := testkit.Interrupt(r.cmd, r.tree); err != nil {
		_, _ = r.end(0)
		fatalf(t, "stopbudget: signal: %v", err)
	}
	code, markers = r.end(90 * time.Second)
	return time.Since(began), code, r.stdout.String() + r.stderr.String(), markers
}

// A stopped suite that reported a 32s teardown budget, its fixture's 1s
// Timeout and its 1s SetupTimeout with the headroom, and lingers past it is
// killed once that budget is spent, not the default's 5m30s.
func (s *CanaryBudgetsTestSuite) TestAStoppedSuiteIsKilledAtTheBudgetItReported(t *gotest.T) {
	took, _, out, markers := s.budgetStopped(t, "STOPBUDGET_LINGER=75s")
	if slices.Contains(markers, "lingered") || took < 30*time.Second || took > 60*time.Second {
		t.Errorf("stopbudget: the suite was not killed at its 32s teardown budget (the run ended %s after the stop, markers %v):\n%s", took, markers, out)
	}
}

// A stopped suite whose reported teardown budget outlasts its 40s linger is
// left to end on its own.
func (s *CanaryBudgetsTestSuite) TestAStoppedSuiteWithinItsBudgetEndsOnItsOwn(t *gotest.T) {
	took, _, out, markers := s.budgetStopped(t, "STOPBUDGET_LINGER=40s", "STOPBUDGET_TIMEOUT=2m")
	if !slices.Contains(markers, "lingered") {
		t.Errorf("stopbudget: the suite was killed before its teardown budget was spent (the run ended %s after the stop, markers %v):\n%s", took, markers, out)
	}
}
