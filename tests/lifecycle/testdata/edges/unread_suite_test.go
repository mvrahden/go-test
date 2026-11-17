package edges

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// UnreadTestSuite reads three of the four sections its baseline holds, or
// two when SNAP_SKIP gates one method out: the fourth belongs to a test that
// is gone.
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

// TestGated skips itself under SNAP_SKIP, as an environment gate would.
func (s *UnreadTestSuite) TestGated(t *gotest.T) {
	if os.Getenv("SNAP_SKIP") != "" {
		t.Skipf("gated")
	}
	gotest.MatchSnapshot(t, "gated")
}
