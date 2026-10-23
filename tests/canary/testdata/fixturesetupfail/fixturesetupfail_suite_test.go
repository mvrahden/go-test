package fixturesetupfail

import "github.com/mvrahden/go-test/pkg/gotest"

type NeverOpensTestSuite struct {
	Half *HalfFixture
}

func (s *NeverOpensTestSuite) TestNever(t *gotest.T) { mark("never") }
