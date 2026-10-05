package wrapped

import "github.com/mvrahden/go-test/pkg/gotest"

type BoundTestSuite struct {
	Ledger *LedgerFixture
}

func (s *BoundTestSuite) TestUses(t *gotest.T) {}

type UnboundTestSuite struct{}

func (s *UnboundTestSuite) TestAlone(t *gotest.T) {}
