package TestResolve_SharedFixture_EachHookElsewhere

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"testpkg/extfixtures"
)

type QueueTestSuite struct {
	Queue *extfixtures.QueueSharedFixture
}

func (s *QueueTestSuite) TestA(t *gotest.T) {}
