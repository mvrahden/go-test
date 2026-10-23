package fixareleasecancel

import "github.com/mvrahden/go-test/pkg/gotest"

// ReleaseTestSuite binds both fixtures.
type ReleaseTestSuite struct {
	Watch *WatchSharedFixture
	Retry *RetrySharedFixture
}

func (s *ReleaseTestSuite) TestNeverRuns(t *gotest.T) { mark("suite ran") }
