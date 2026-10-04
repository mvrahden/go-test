package testmainuser

import "github.com/mvrahden/go-test/pkg/gotest"

type InTestSuite struct {
	Ledger *LedgerFixture
}

func (s *InTestSuite) TestWrite(t *gotest.T) {}
