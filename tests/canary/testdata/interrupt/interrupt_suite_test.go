package interrupt

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldingTestSuite holds the run open until it is stopped.
type HoldingTestSuite struct {
	Ledger *LedgerFixture
}

func (s *HoldingTestSuite) TestHolds(t *gotest.T) {
	logLine("running")
	select {
	case <-time.After(time.Minute):
	case <-t.Context().Done():
	}
}
