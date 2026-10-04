package withtestmain_wrapped

import (
	"context"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }

type LedgerTestSuite struct {
	Ledger *LedgerFixture
}

func (s *LedgerTestSuite) TestRead(t *gotest.T) {}

// verify stands in for goleak.VerifyTestMain: it runs the tests itself.
func verify(m interface{ Run() int }) { _ = m.Run() }

func TestMain(m *testing.M) { verify(gotestruntime.M(m)) }
