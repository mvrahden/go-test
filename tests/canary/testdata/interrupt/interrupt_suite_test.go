package interrupt

import (
	"os"
	"sync"
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

// FuzzingTestSuite fuzzes against the fixture. With INTERRUPT_FUZZ=1 each
// process marks once that it is fuzzing, so a stop can wait for the engine.
type FuzzingTestSuite struct {
	Ledger *LedgerFixture
}

var fuzzingOnce sync.Once

func (s *FuzzingTestSuite) FuzzSpins(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		if os.Getenv("INTERRUPT_FUZZ") == "1" {
			fuzzingOnce.Do(func() { logLine("running") })
			time.Sleep(time.Millisecond)
		}
	})
}
