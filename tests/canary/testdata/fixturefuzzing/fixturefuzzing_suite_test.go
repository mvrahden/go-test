package fixturefuzzing

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BoundFuzzTestSuite binds a package fixture and fuzzes against it: the seed
// replay runs after the Test function and must still find the fixture up.
type BoundFuzzTestSuite struct {
	Ledger *LedgerFixture
}

func (s *BoundFuzzTestSuite) TestFixtureAlive(t *gotest.T) {
	_, err := os.Stat(AlivePath())
	gotest.NoError(t, err)
}

func (s *BoundFuzzTestSuite) FuzzSeesFixture(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		got, err := os.ReadFile(AlivePath())
		gotest.NoError(t, err, "fixture released before the seed replay")
		gotest.Equal(t, "alive", string(got))
		gotest.NotEmpty(t, in)
	})
}
