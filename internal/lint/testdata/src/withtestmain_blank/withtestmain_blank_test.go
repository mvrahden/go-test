package withtestmain_blank

import (
	"context"
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	_ "github.com/mvrahden/go-test/pkg/gotestruntime"
)

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }

type LedgerTestSuite struct {
	Ledger *LedgerFixture
}

func (s *LedgerTestSuite) TestRead(t *gotest.T) {}

// A blank import gives the fix no name to call the runtime by.
func TestMain(m *testing.M) { os.Exit(m.Run()) } // want `TestMain must run the tests through gotestruntime`
