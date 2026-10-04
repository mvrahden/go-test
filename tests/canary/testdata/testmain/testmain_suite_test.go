package testmain

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SeededTestSuite replays its fuzz seeds after its Test function, against the
// same fixture.
type SeededTestSuite struct {
	Ledger *LedgerFixture
}

func (s *SeededTestSuite) TestUp(t *gotest.T) { gotest.True(t, Up, "fixture down") }

func (s *SeededTestSuite) FuzzUp(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) { gotest.True(t, Up, "fixture down") })
}

type PanickingTestSuite struct {
	Ledger *LedgerFixture
}

func (s *PanickingTestSuite) TestUp(t *gotest.T) {
	if os.Getenv("TESTMAIN_PANIC") == "1" {
		panic("method panic")
	}
	gotest.True(t, Up, "fixture down")
}

type UnboundTestSuite struct{}

func (s *UnboundTestSuite) TestAlone(t *gotest.T) {}
