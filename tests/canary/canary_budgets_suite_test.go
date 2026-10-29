package canary_test //nolint:fail-guard

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CanaryBudgetsTestSuite runs the fixtures whose verdicts are budgets and
// deadlines: teardown budgets, the setup wait, the default deadline.
// Sequential: each method spawns a full pipeline run whose verdict reads the clock.
type CanaryBudgetsTestSuite struct{ canary }

func (s *CanaryBudgetsTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Slowest method ~33s locally, ~50s estimated for Windows CI, setup ~13s; budgets are twice that, rounded up.
	return gotest.SuiteConfig{Timeout: 2 * time.Minute, SetupTimeout: 30 * time.Second}
}

func (s *CanaryBudgetsTestSuite) BeforeAll(t *gotest.T) { s.open(t) }

func (s *CanaryBudgetsTestSuite) AfterAll(t *gotest.T) {}

// A shared fixture started for the exclusive tail is released under the
// teardown budget its declared 2s Timeout gives: its AfterAll, which never
// returns, is force-killed once 32s have passed, not the default's 2m30s.
func (s *CanaryBudgetsTestSuite) TestALateFixtureIsReleasedUnderItsDeclaredBudget(t *gotest.T) {
	// Built first, in the same canary dir, so the clock reads the release.
	dir := t.TempDir()
	if r := s.runWith(t, "latebudget", dir, nil, "-run=^$"); r.code != 0 {
		fatalf(t, "prepare latebudget: exit %d: %s", r.code, r.stderr)
	}
	began := time.Now()
	r := s.checkIn(t, "latebudget", dir)
	if !strings.Contains(r.stderr, "force-killed after 32s;") {
		t.Errorf("latebudget: the late fixture was not released under its 32s teardown budget:\n%s", r.stderr)
	}
	if took := time.Since(began); took > 90*time.Second {
		t.Errorf("latebudget: the run took %s; the release outlived the fixture's 32s teardown budget", took)
	}
}

// A shared fixture whose attempts and the release between them each take 1.6s
// of its 2s Timeout is waited for: the budget counts the release.
func (s *CanaryBudgetsTestSuite) TestTheSetupWaitCountsTheReleaseBetweenAttempts(t *gotest.T) {
	s.check(t, "slowretry")
}

// A context the default deadline ended in a suite that declares no Timeout
// is named once, with its method, and the method still passes; a method
// past a declared Timeout fails by it and gets no such line.
func (s *CanaryBudgetsTestSuite) TestAContextTheDefaultDeadlineEndedIsNamed(t *gotest.T) {
	r := s.check(t, "defaultdeadline")
	const line = "t.Context() ended at the default 30s deadline; the suite declares no Timeout"
	waiting := streamOutput(r.stream, "TestWaitingTestSuite/TestWaitsOnItsContext")
	if n := strings.Count(waiting, "TestWaitingTestSuite/TestWaitsOnItsContext: "+line); n != 1 {
		t.Errorf("defaultdeadline: the waiting method was named %d times, want once:\n%s", n, waiting)
	}
	if n := strings.Count(r.stream, "t.Context() ended at the default "); n != 1 {
		t.Errorf("defaultdeadline: the default deadline was named %d times in the run, want once:\n%s", n, r.stream)
	}
}

// GOTEST_TIMEOUT_SCALE=2 doubles the 1s budgets: the method and fixture
// taking 1.5s pass, the ones taking 2.5s fail naming 2s, and a method finds
// the scale gone from its environment. A scale that is no number fails both
// suites before anything runs.
func (s *CanaryBudgetsTestSuite) TestBudgetsScaledForASlowMachine(t *gotest.T) {
	for _, c := range []struct{ key, scale string }{{"timeoutscale", "2"}, {"timeoutscalefast", "fast"}} {
		want := s.expected[c.key]
		r := s.runIn(t, "", "./testdata/timeoutscale/", c.key, t.TempDir(), []string{"GOTEST_TIMEOUT_SCALE=" + c.scale})
		if r.code != want.exit {
			t.Errorf("%s: exit code %d, want %d", c.key, r.code, want.exit)
		}
		if strings.Join(r.rows, "\n") != strings.Join(want.rows, "\n") {
			t.Errorf("%s: verdicts differ from the golden list\n got:\n  %s\n want:\n  %s",
				c.key, strings.Join(r.rows, "\n  "), strings.Join(want.rows, "\n  "))
		}
		if c.scale != "2" {
			continue
		}
		out := r.stream + r.stderr
		if !strings.Contains(out, "exceeded its configured Timeout of 2s") {
			t.Errorf("%s: no overrun names the scaled 2s Timeout", c.key)
		}
		if !strings.Contains(out, "SlowerFixture.BeforeAll exceeded its configured Timeout of 2s") {
			t.Errorf("%s: the fixture's overrun does not name the scaled 2s Timeout", c.key)
		}
	}
}
