package fixtureorder

import "github.com/mvrahden/go-test/pkg/gotest"

type AheadTestSuite struct {
	Broken *BrokenFixture
}

func (s *AheadTestSuite) TestBody(t *gotest.T) { mark("ahead body") }

type BehindTestSuite struct {
	Behind *BehindFixture
}

func (s *BehindTestSuite) TestBody(t *gotest.T) { mark("behind body") }
