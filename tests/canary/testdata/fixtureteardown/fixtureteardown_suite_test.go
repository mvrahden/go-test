package fixtureteardown

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FirstBoundTestSuite and SecondBoundTestSuite share one package fixture, each
// in its own process with its own copy; every process tears its copy down.
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
