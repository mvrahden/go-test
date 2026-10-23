package benchpanic

import "github.com/mvrahden/go-test/pkg/gotest"

// PanickingBenchTestSuite: one benchmark panics, the next still measures.
type PanickingBenchTestSuite struct {
	Held *HeldFixture
}

func (s *PanickingBenchTestSuite) AfterAll(t *gotest.T) { mark("suite.AfterAll") }

func (s *PanickingBenchTestSuite) BenchmarkPanics(b *gotest.B) {
	mark("panics")
	panic("the benchmark panics on purpose")
}

func (s *PanickingBenchTestSuite) BenchmarkStillRuns(b *gotest.B) {
	mark("still runs")
	for b.Loop() {
		_ = 1 + 1
	}
}
