package fixabenchbudget

import "github.com/mvrahden/go-test/pkg/gotest"

// AFreeTestSuite binds nothing and benchmarks first, so the fixture's window
// opens only at the slot after it.
type AFreeTestSuite struct{}

func (s *AFreeTestSuite) BenchmarkFree(b *gotest.B) {
	for b.Loop() {
	}
}

// LateTestSuite benchmarks against the late fixture.
type LateTestSuite struct {
	Late *LateSharedFixture
}

func (s *LateTestSuite) BenchmarkStateArrived(b *gotest.B) {
	gotest.Equal(b, "up", s.Late.State)
	for b.Loop() {
	}
}
