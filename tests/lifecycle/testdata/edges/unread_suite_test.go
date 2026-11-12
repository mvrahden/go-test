package edges

import "github.com/mvrahden/go-test/pkg/gotest"

// UnreadTestSuite reads two of the three sections its baseline holds: the
// third belongs to a test that is gone.
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
