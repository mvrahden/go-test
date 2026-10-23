package fixturehookfail

import "github.com/mvrahden/go-test/pkg/gotest"

type HookFailTestSuite struct {
	Flaky *FlakyFixture
}

func (s *HookFailTestSuite) TestFirst(t *gotest.T)  { mark("first") }
func (s *HookFailTestSuite) TestSecond(t *gotest.T) { mark("second") }
