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
