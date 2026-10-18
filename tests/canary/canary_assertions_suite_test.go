package canary_test //nolint:fail-guard

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A poll that started before waitFor and returns within the next tick is
// judged by its result, for Eventually and Consistently alike.
func (s *CanaryTestSuite) TestALatePollIsJudgedByItsResult(t *gotest.T) {
	s.check(t, "latepolls")
	if strings.Contains(s.lastStream, "poll still running") {
		t.Errorf("latepolls: a poll started before waitFor was failed as still running")
	}
}

// A fuzz execution whose BeforeEach skips is skipped: its body never runs and
// its AfterEach still does.
func (s *CanaryTestSuite) TestAFuzzExecutionSkippedByBeforeEachStillTearsDown(t *gotest.T) {
	dir := s.check(t, "fuzzskiphook")
	got, _ := os.ReadFile(filepath.Join(dir, "markers"))
	if strings.Contains(string(got), "body ran\n") {
		t.Errorf("fuzzskiphook: the body ran after BeforeEach skipped:\n%s", got)
	}
	if !strings.Contains(string(got), "after skip\n") {
		t.Errorf("fuzzskiphook: AfterEach did not run after a BeforeEach that skipped:\n%s", got)
	}
}
