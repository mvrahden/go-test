package teardownfailing

import "github.com/mvrahden/go-test/pkg/gotest"

// TeardownFailingTestSuite passes; what fails is the shared fixture it reads.
type TeardownFailingTestSuite struct {
	Probe *ReleaseSharedFixture
}

func (s *TeardownFailingTestSuite) TestFixtureIsUp(t *gotest.T) {
	gotest.Equal(t, "up", s.Probe.State)
}
