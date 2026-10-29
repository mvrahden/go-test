package stoppable

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"shutdownmod/fixtures"
)

// StoppableTestSuite announces that it is running and then sleeps until its
// context ends, so a stop ends the method at once instead of after its grace.
type StoppableTestSuite struct {
	Probe *fixtures.ProbeSharedFixture
}

func (s *StoppableTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *StoppableTestSuite) TestSleeps(t *gotest.T) {
	gotest.NoError(t, os.WriteFile(filepath.Join(s.Probe.Dir, "test-running"), []byte("running"), 0o600))
	select {
	case <-t.Context().Done():
	case <-time.After(60 * time.Second):
	}
}
