package fixahang

import "github.com/mvrahden/go-test/pkg/gotest"

// HangTestSuite reads the fixture that never comes up.
type HangTestSuite struct {
	Hang *HangSharedFixture
}

func (s *HangTestSuite) TestNeverRuns(t *gotest.T) { mark("suite ran") }
