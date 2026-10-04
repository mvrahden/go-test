package testmainboth_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen/testdata_e2e/testmain_both"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExtTestSuite struct {
	Ledger *testmainboth.LedgerFixture
}

func (s *ExtTestSuite) TestRead(t *gotest.T) {}
