// Package snapunread reads two of the three sections its committed baseline
// holds: the third belongs to a test that is gone.
package snapunread

import "github.com/mvrahden/go-test/pkg/gotest"

type UnreadTestSuite struct{}

func (s *UnreadTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *UnreadTestSuite) TestKept(t *gotest.T) {
	gotest.MatchSnapshot(t, "kept")
}

func (s *UnreadTestSuite) TestOther(t *gotest.T) {
	gotest.MatchSnapshot(t, "other")
}
