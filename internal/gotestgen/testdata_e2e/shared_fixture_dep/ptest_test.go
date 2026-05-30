package sharedfixdep

import "github.com/mvrahden/go-test/pkg/gotest"

type DepTestSuite struct {
	Beta *BetaSharedFixture
}

func (s *DepTestSuite) TestBetaReady(t *gotest.T) { DoWork() }
