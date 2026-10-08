package holds

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SeededTestSuite replays its fuzz seeds in a top-level function of their
// own, which holds the fixture again.
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
	if os.Getenv("HOLDS_PANIC") == "1" {
		panic("method panic")
	}
	gotest.True(t, Up, "fixture down")
}

// BenchmarkPanics panics with HOLDS_BENCH_PANIC=1: a sub-benchmark runs only
// its own cleanups on a panic, not its parent's.
func (s *PanickingTestSuite) BenchmarkPanics(b *gotest.B) {
	if os.Getenv("HOLDS_BENCH_PANIC") == "1" {
		panic("benchmark panic")
	}
	for b.Loop() {
	}
}

type UnboundTestSuite struct{}

func (s *UnboundTestSuite) TestAlone(t *gotest.T) {}
