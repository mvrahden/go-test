package asyncdone_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// NeverDoneTestSuite has an async method that returns without calling done.
type NeverDoneTestSuite struct{}

func (s *NeverDoneTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 100 * time.Millisecond}
}

func (s *NeverDoneTestSuite) TestNeverDoneAsync(t *gotest.T, done func()) {}

// RacingTestSuite runs async methods that never call done next to a method
// that overruns its Timeout: only that one is still running at the deadline.
type RacingTestSuite struct{}

func (s *RacingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 100 * time.Millisecond, Parallel: true}
}

func (s *RacingTestSuite) TestFirstNeverDoneAsync(t *gotest.T, done func()) {}

func (s *RacingTestSuite) TestSecondNeverDoneAsync(t *gotest.T, done func()) {}

func (s *RacingTestSuite) TestOverruns(t *gotest.T) { time.Sleep(300 * time.Millisecond) }
