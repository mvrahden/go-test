package slow

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"shutdownmod/fixtures"
)

// SlowTestSuite announces that it is running and then sleeps, so the test
// that spawned the run can interrupt it mid-method.
type SlowTestSuite struct {
	Probe *fixtures.ProbeSharedFixture
}

func (s *SlowTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *SlowTestSuite) TestSleeps(t *gotest.T) {
	gotest.NoError(t, os.WriteFile(filepath.Join(s.Probe.Dir, "test-running"), []byte("running"), 0o600))
	time.Sleep(60 * time.Second)
}
