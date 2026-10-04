package withtestmain_alias

import (
	"context"
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	rt "github.com/mvrahden/go-test/pkg/gotestruntime"
)

var _ = rt.Main

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }

type LedgerTestSuite struct {
	Ledger *LedgerFixture
}

func (s *LedgerTestSuite) TestRead(t *gotest.T) {}

// The runtime is already imported under an alias; the fix uses it.
func TestMain(tm *testing.M) { // want `TestMain must run the tests through gotestruntime`
	code := tm.Run()
	os.Exit(code)
}
