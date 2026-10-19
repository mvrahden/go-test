package canary_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// A seed of the wrong type fails its target at the f.Add call, line 10 of
// the fixture, not inside gotest's or the generated code.
func (s *CanaryTestSuite) TestASeedOfTheWrongTypeIsReportedAtItsAdd(t *gotest.T) {
	r := s.check(t, "fuzzseedtype")
	if !strings.Contains(r.stream, "fuzzseedtype_suite_test.go:10: seed #1: value 1: f.Add was given int64, but this fuzz target takes int") {
		t.Errorf("fuzzseedtype: the seed failure is not reported at the f.Add call")
	}
	if strings.Contains(r.stream, "f.go:") {
		t.Errorf("fuzzseedtype: the seed failure is reported inside gotest's file")
	}
}
