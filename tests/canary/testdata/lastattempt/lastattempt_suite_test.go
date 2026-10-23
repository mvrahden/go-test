package lastattempt

import "github.com/mvrahden/go-test/pkg/gotest"

// DownTestSuite binds the fixture and asserts nothing about it.
type DownTestSuite struct {
	Down *DownSharedFixture
}

func (s *DownTestSuite) TestPassesWithoutIt(t *gotest.T) {}
