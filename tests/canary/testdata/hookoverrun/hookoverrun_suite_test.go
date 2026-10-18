package hookoverrun

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type OverrunTestSuite struct {
	Slow *SlowFixture
}

func (s *OverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

func (s *OverrunTestSuite) TestSlow(t *gotest.T) { mark("body") }
