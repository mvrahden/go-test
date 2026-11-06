package sharedstop

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldTestSuite holds until its context ends, so a stop ends it at once.
type HoldTestSuite struct {
	Asked *AskedSharedFixture
}

func (s *HoldTestSuite) TestHolds(t *gotest.T) {
	mark("running")
	<-t.Context().Done()
}

// IgnoreTestSuite ignores the stop beside a package fixture whose release
// takes 10s, so its process outlasts a second stop too.
type IgnoreTestSuite struct {
	Asked *AskedSharedFixture
	Hold  *HoldFixture
}

func (s *IgnoreTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *IgnoreTestSuite) TestIgnores(t *gotest.T) {
	mark("running")
	time.Sleep(time.Minute)
}
