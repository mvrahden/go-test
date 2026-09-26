package fixtureteardown

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FirstBoundTestSuite and SecondBoundTestSuite share one package fixture; the
// fixture's AfterAll must run once the last suite process is done with it.
type FirstBoundTestSuite struct {
	Ledger *LedgerFixture
}

func (s *FirstBoundTestSuite) TestFixtureAlive(t *gotest.T) {
	_, err := os.Stat(AlivePath())
	gotest.NoError(t, err)
}

func (s *FirstBoundTestSuite) BenchmarkFixtureAlive(b *gotest.B) {
	for b.Loop() {
		_, err := os.Stat(AlivePath())
		gotest.NoError(b, err)
	}
}

type SecondBoundTestSuite struct {
	Ledger *LedgerFixture
}

func (s *SecondBoundTestSuite) TestFixtureAlive(t *gotest.T) {
	_, err := os.Stat(AlivePath())
	gotest.NoError(t, err)
}
