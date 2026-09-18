package failfast

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
	"shutdownmod/fixtures"
)

// FailFastTestSuite fails its first method; with FailFast the second must
// never run, and the shared fixture must still be torn down.
type FailFastTestSuite struct {
	Probe *fixtures.ProbeSharedFixture
}

func (s *FailFastTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.FailFast = true
	return cfg
}

func (s *FailFastTestSuite) TestFirst(t *gotest.T) { gotest.Fail(t, "fails on purpose") }

func (s *FailFastTestSuite) TestSecond(t *gotest.T) {
	gotest.NoError(t, os.WriteFile(filepath.Join(s.Probe.Dir, "second-ran"), []byte("ran"), 0o600))
}
