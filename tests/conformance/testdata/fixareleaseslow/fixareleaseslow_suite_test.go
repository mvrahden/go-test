package fixareleaseslow

import "github.com/mvrahden/go-test/pkg/gotest"

// SlowTestSuite binds the fixture and would pass without its release.
type SlowTestSuite struct {
	Slow *SlowReleaseFixture
}

func (s *SlowTestSuite) TestPasses(t *gotest.T) { mark("suite ran") }
