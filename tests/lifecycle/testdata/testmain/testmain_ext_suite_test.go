package testmain_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/lifecycle/testdata/testmain"
)

// ExtTestSuite binds the fixture from the external test package, which shares
// the binary and its one TestMain.
type ExtTestSuite struct {
	Ledger *testmain.LedgerFixture
}

func (s *ExtTestSuite) TestUp(t *gotest.T) { gotest.True(t, testmain.Up, "fixture down") }
