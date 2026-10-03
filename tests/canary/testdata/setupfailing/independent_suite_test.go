package setupfailing

import "github.com/mvrahden/go-test/pkg/gotest"

// IndependentTestSuite reads no fixture, so the one that fails to come up
// is none of its business: it runs and reports.
type IndependentTestSuite struct{}

func (s *IndependentTestSuite) TestRuns(t *gotest.T) {
	gotest.True(t, true)
}

func (s *IndependentTestSuite) BenchmarkRuns(b *gotest.B) {
	for b.Loop() {
		_ = 1 + 1
	}
}
