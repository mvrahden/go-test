package groups

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// e2e is the group of the suites that run apart, together.
const e2e = "e2e"

type ADefaultTestSuite struct{}

func (s *ADefaultTestSuite) TestRuns(t *gotest.T) { span("default.a", 500*time.Millisecond) }

type BDefaultTestSuite struct{}

func (s *BDefaultTestSuite) TestRuns(t *gotest.T) { span("default.b", 500*time.Millisecond) }

type E2EOneTestSuite struct{}

func (s *E2EOneTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{ExclusiveGroup: e2e}
}

func (s *E2EOneTestSuite) TestRuns(t *gotest.T) { span("e2e.one", time.Second) }

type E2ETwoTestSuite struct{}

func (s *E2ETwoTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.ExclusiveGroup = "e2e"
	return cfg
}

func (s *E2ETwoTestSuite) TestRuns(t *gotest.T) { span("e2e.two", time.Second) }

// E2EAloneTestSuite is exclusive within the e2e group.
type E2EAloneTestSuite struct{}

func (s *E2EAloneTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{ExclusiveGroup: e2e, Exclusive: true}
}

func (s *E2EAloneTestSuite) TestRuns(t *gotest.T) { span("e2e.alone", 200*time.Millisecond) }

// LoadTestSuite is in the group after e2e by name.
type LoadTestSuite struct{}

func (s *LoadTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{ExclusiveGroup: "load"}
}

func (s *LoadTestSuite) TestRuns(t *gotest.T) { span("load", 200*time.Millisecond) }
