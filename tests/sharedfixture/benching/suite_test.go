package benching

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// The two suites need disjoint shared fixtures, so a serial bench run must
// close Beta's window after the first slot and open Delta's just before the
// second: `gotest bench ./tests/sharedfixture/...` passing drives the
// per-slot JIT window scheduling end to end.

type BetaBenchTestSuite struct {
	Beta *fixtures.BetaSharedFixture
}

func (s *BetaBenchTestSuite) BenchmarkLabel(b *gotest.B) {
	label := s.Beta.Label // hoisted: the fixture is the setup, not the measurement
	for b.Loop() {
		if !strings.HasPrefix(label, "beta") {
			b.Errorf("Beta not hydrated: %q", label)
		}
	}
}

type DeltaBenchTestSuite struct {
	Delta *fixtures.DeltaSharedFixture
}

func (s *DeltaBenchTestSuite) BenchmarkStamp(b *gotest.B) {
	stamp := s.Delta.Stamp // hoisted: the fixture is the setup, not the measurement
	for b.Loop() {
		if stamp != "delta-shared" {
			b.Errorf("Delta not hydrated: %q", stamp)
		}
	}
}
