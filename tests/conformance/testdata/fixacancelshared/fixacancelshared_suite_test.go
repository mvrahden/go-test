package fixacancelshared

import "github.com/mvrahden/go-test/pkg/gotest"

// CancelTestSuite binds the fixture set up last.
type CancelTestSuite struct {
	Later *LaterSharedFixture
}

func (s *CancelTestSuite) TestNeverRuns(t *gotest.T) { mark("suite ran") }
