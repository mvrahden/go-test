package canary_test //nolint:fail-guard

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A shared fixture started for the exclusive tail is released under the
// teardown budget its declared 2s Timeout gives: its AfterAll, which never
// returns, is force-killed once 32s have passed, not the default's 2m30s.
func (s *CanaryTestSuite) TestALateFixtureIsReleasedUnderItsDeclaredBudget(t *gotest.T) {
	// Built first, in the same canary dir, so the clock reads the release.
	dir := t.TempDir()
	if code, _ := s.runWith(t, "latebudget", dir, nil, "-run=^$"); code != 0 {
		fatalf(t, "prepare latebudget: exit %d: %s", code, s.lastStderr)
	}
	began := time.Now()
	s.checkIn(t, "latebudget", dir)
	if !strings.Contains(s.lastStderr, "force-killed after 32s;") {
		t.Errorf("latebudget: the late fixture was not released under its 32s teardown budget:\n%s", s.lastStderr)
	}
	if took := time.Since(began); took > 90*time.Second {
		t.Errorf("latebudget: the run took %s; the release outlived the fixture's 32s teardown budget", took)
	}
}

// A shared fixture whose attempts and the release between them each take 1.6s
// of its 2s Timeout is waited for: the budget counts the release.
func (s *CanaryTestSuite) TestTheSetupWaitCountsTheReleaseBetweenAttempts(t *gotest.T) {
	s.check(t, "slowretry")
}

// A context the default deadline ended in a suite that declares no Timeout
// is named once, with its method, and the method still passes; a method
// past a declared Timeout fails by it and gets no such line.
func (s *CanaryTestSuite) TestAContextTheDefaultDeadlineEndedIsNamed(t *gotest.T) {
	s.check(t, "defaultdeadline")
	const line = "t.Context() ended at the default 30s deadline; the suite declares no Timeout"
	waiting := streamOutput(s.lastStream, "TestWaitingTestSuite/TestWaitsOnItsContext")
	if n := strings.Count(waiting, "TestWaitingTestSuite/TestWaitsOnItsContext: "+line); n != 1 {
		t.Errorf("defaultdeadline: the waiting method was named %d times, want once:\n%s", n, waiting)
	}
	if n := strings.Count(s.lastStream, "t.Context() ended at the default "); n != 1 {
		t.Errorf("defaultdeadline: the default deadline was named %d times in the run, want once:\n%s", n, s.lastStream)
	}
}
