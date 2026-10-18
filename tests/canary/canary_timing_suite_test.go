package canary_test //nolint:fail-guard

import "github.com/mvrahden/go-test/pkg/gotest"

// CanaryTimingTestSuite runs the fixture packages whose verdicts bound how
// long a run takes, one at a time and alone, so no other run's load shifts
// the clock they read.
// Sequential: every method times a full pipeline run.
type CanaryTimingTestSuite struct {
	canary
}

func (s *CanaryTimingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true}
}

func (s *CanaryTimingTestSuite) BeforeAll(t *gotest.T) { s.setUp(t) }

func (s *CanaryTimingTestSuite) AfterAll(t *gotest.T) {}
