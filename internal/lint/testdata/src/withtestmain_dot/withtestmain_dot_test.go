package withtestmain_dot

import (
	"context"
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	. "github.com/mvrahden/go-test/pkg/gotestruntime"
)

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }

type LedgerTestSuite struct {
	Ledger *LedgerFixture
}

func (s *LedgerTestSuite) TestRead(t *gotest.T) {}

// The runtime dot-imported routes the tests through it.
func TestMain(m *testing.M) { os.Exit(Main(m)) }
