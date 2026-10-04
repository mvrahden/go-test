package withtestmain_runner

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

// verify stands in for goleak.VerifyTestMain: it takes m as a Run interface,
// so the wrapper fits.
func verify(m interface{ Run() int }) { _ = m.Run() }

// inspect takes the concrete *testing.M; the wrapper would not compile there.
func inspect(m *testing.M) {}

func TestMain(m *testing.M) { // want `TestMain must run the tests through gotestruntime`
	inspect(m)
	verify(m)
}
