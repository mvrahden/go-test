package slowteardown

import "github.com/mvrahden/go-test/pkg/gotest"

type LedgerTestSuite struct {
	Ledger *LedgerFixture
}

func (s *LedgerTestSuite) TestQuick(t *gotest.T) {}
