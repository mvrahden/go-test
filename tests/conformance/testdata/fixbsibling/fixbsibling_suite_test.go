package fixbsibling

import "github.com/mvrahden/go-test/pkg/gotest"

// SiblingsTestSuite never runs: one of its fixtures does not come up in time.
type SiblingsTestSuite struct {
	Hung    *HungSharedFixture
	Healthy *HealthySharedFixture
}

func (s *SiblingsTestSuite) TestNeverRuns(t *gotest.T) {
	gotest.Fail(t, "a suite without its fixture ran")
}
