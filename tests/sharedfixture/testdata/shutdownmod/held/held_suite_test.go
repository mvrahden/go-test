package held

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"shutdownmod/fixtures"
)

// HoldFixture is a package fixture, so a stopped method holds the suite
// process for its grace instead of letting it end at once.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(context.Context) error { return nil }

// HeldTestSuite announces that it is running and then sleeps, ignoring the
// stop, while it holds a package fixture beside the shared one.
type HeldTestSuite struct {
	Probe *fixtures.ProbeSharedFixture
	Hold  *HoldFixture
}

func (s *HeldTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *HeldTestSuite) TestSleeps(t *gotest.T) {
	gotest.NoError(t, os.WriteFile(filepath.Join(s.Probe.Dir, "test-running"), []byte("running"), 0o600))
	time.Sleep(60 * time.Second)
}
