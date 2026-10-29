package sharedteardownsibling

import "github.com/mvrahden/go-test/pkg/gotest"

type DownTestSuite struct {
	Down *DownSharedFixture
}

func (s *DownTestSuite) TestNeverRuns(t *gotest.T) { gotest.Fail(t, "a suite without its fixture ran") }

type StuckTestSuite struct {
	Stuck *StuckSharedFixture
}

func (s *StuckTestSuite) TestRuns(t *gotest.T) {}
