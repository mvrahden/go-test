package fixtureafterfail

import "github.com/mvrahden/go-test/pkg/gotest"

type AfterFailTestSuite struct {
	Leaky *LeakyFixture
}

func (s *AfterFailTestSuite) TestBody(t *gotest.T) {}
