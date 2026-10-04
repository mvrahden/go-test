package testmainxonly_test

import (
	"github.com/mvrahden/go-test/internal/gotestgen/testdata_e2e/testmain_xonly"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExtTestSuite struct {
	Ledger *testmainxonly.LedgerFixture
}

func (s *ExtTestSuite) TestRead(t *gotest.T) {}
