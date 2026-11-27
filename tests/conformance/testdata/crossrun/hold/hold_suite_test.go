package hold

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldTestSuite is the default group's Exclusive suite.
type HoldTestSuite struct{}

func (s *HoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 90 * time.Second}
}

func (s *HoldTestSuite) TestHolds(t *gotest.T) {
	t.AssertsBy("it stamps markers the run under test reads back")
	span("hold")
}

// E2EHoldTestSuite is a suite of the e2e group.
type E2EHoldTestSuite struct{}

func (s *E2EHoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{ExclusiveGroup: "e2e", Timeout: 90 * time.Second}
}

func (s *E2EHoldTestSuite) TestHolds(t *gotest.T) {
	t.AssertsBy("it stamps markers the run under test reads back")
	span("e2e")
}
