package stopshared

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldTestSuite reads the fixture and holds until the run is stopped.
type HoldTestSuite struct {
	Fixture *SlowReleaseSharedFixture
}

func (s *HoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *HoldTestSuite) TestHolds(t *gotest.T) {
	mark("suite holds")
	<-t.Context().Done()
}
