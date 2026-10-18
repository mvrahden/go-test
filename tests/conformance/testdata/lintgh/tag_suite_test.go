package lintgh

import "github.com/mvrahden/go-test/pkg/gotest"

// TagTestSuite binds the shared fixture.
type TagTestSuite struct {
	Tag *TagSharedFixture
}

func (s *TagTestSuite) TestRuns(t *gotest.T) {}
