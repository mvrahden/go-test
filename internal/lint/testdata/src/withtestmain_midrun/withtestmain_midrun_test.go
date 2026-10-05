package withtestmain_midrun

import (
	"context"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }

type LedgerTestSuite struct {
	Ledger *LedgerFixture
}

func (s *LedgerTestSuite) TestRead(t *gotest.T) {}

func cleanup() {}

// A discarded m.Run() followed by more work has no rewrite that keeps the
// control flow: reported, not rewritten.
func TestMain(m *testing.M) { // want `TestMain must run the tests through gotestruntime`
	m.Run()
	cleanup()
}
