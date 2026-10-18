package fixabench

import "github.com/mvrahden/go-test/pkg/gotest"

// HangTestSuite benchmarks against the fixture that never comes up.
type HangTestSuite struct {
	Hang *HangSharedFixture
}

func (s *HangTestSuite) BenchmarkNeverRuns(b *gotest.B) {
	mark("bench ran")
	for b.Loop() {
	}
}

// AFreeTestSuite binds nothing and benchmarks first, so the fixture's window
// opens only at the slot after it.
type AFreeTestSuite struct{}

func (s *AFreeTestSuite) BenchmarkFree(b *gotest.B) {
	mark("free ran")
	for b.Loop() {
	}
}
