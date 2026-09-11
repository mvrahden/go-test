package quick

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"shutdownmod/fixtures"
)

// QuickTestSuite passes at once, so the run it belongs to is in its
// teardown almost immediately after it starts.
type QuickTestSuite struct {
	Probe *fixtures.ProbeSharedFixture
}

func (s *QuickTestSuite) TestPasses(t *gotest.T) {
	gotest.NotEmpty(t, s.Probe.Dir)
}
