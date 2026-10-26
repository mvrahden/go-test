package overrun_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// RacingTestSuite runs methods that finish in time next to one that overruns
// its Timeout: only that one is still running at the deadline.
type RacingTestSuite struct{}

func (s *RacingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 100 * time.Millisecond, Parallel: true}
}

func (s *RacingTestSuite) TestFirstInTime(t *gotest.T) {}

func (s *RacingTestSuite) TestSecondInTime(t *gotest.T) {}

func (s *RacingTestSuite) TestOverruns(t *gotest.T) { time.Sleep(300 * time.Millisecond) }
