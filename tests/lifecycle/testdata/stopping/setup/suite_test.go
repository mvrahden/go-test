package setup

import "github.com/mvrahden/go-test/pkg/gotest"

type SlowSetupTestSuite struct {
	Container *ContainerFixture
}

func (s *SlowSetupTestSuite) TestUses(t *gotest.T) { logLine("running") }
