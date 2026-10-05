package withtestmain_return

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

// A TestMain that returns exits with m.Run's own code.
func TestMain(m *testing.M) { // want `TestMain must run the tests through gotestruntime`
	m.Run()
}
