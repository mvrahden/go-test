package hydratefail

import "github.com/mvrahden/go-test/pkg/gotest"

// HydrateTestSuite binds the fixture whose Hydrate fails.
type HydrateTestSuite struct {
	Pool *PoolSharedFixture
}

func (s *HydrateTestSuite) TestNeverRuns(t *gotest.T) { mark("suite ran") }
