package holds_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/lifecycle/testdata/holds"
)

// ExtTestSuite binds the fixture from the external test package, which shares
// the binary and the fixture's package variables.
type ExtTestSuite struct {
	Ledger *holds.LedgerFixture
}

func (s *ExtTestSuite) TestUp(t *gotest.T) { gotest.True(t, holds.Up, "fixture down") }
