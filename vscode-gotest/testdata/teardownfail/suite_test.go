package teardownfail_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"gotest.teardownfail"
)

// TeardownFailTestSuite passes. What fails is the shared fixture it reads, and
// only when the environment says so.
type TeardownFailTestSuite struct {
	Probe *teardownfail.ProbeSharedFixture
}

func (s *TeardownFailTestSuite) TestFixtureIsUp(t *gotest.T) {
	gotest.Equal(t, "up", s.Probe.State)
}
