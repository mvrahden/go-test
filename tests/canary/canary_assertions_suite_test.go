package canary_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A poll that started before waitFor is judged by its result, for
// Eventually and Consistently alike, however many ticks it took; a poll
// abandoned past its deadline that panics later is reported to stderr.
func (s *CanaryTestSuite) TestALatePollIsJudgedByItsResult(t *gotest.T) {
	s.check(t, "latepolls")
	if strings.Contains(s.lastStream, "poll still running") {
		t.Errorf("latepolls: a poll started before waitFor was failed as still running")
	}
	if !strings.Contains(s.lastStream, "gotest: a poll abandoned past its deadline panicked: late boom") {
		t.Errorf("latepolls: no stderr line names the abandoned poll's panic")
	}
}
