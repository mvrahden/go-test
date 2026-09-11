package benching_test

import "github.com/mvrahden/go-test/pkg/gotest"

// BenchingTestSuite declares three green benchmarks; a bench run must report
// a result for each, and Context() must be usable inside one.
type BenchingTestSuite struct{}

func (s *BenchingTestSuite) BenchmarkFirst(b *gotest.B) {
	for b.Loop() {
		_ = 1 + 1
	}
}

func (s *BenchingTestSuite) BenchmarkSecond(b *gotest.B) {
	for b.Loop() {
		_ = 2 + 2
	}
}

func (s *BenchingTestSuite) BenchmarkContext(b *gotest.B) {
	if b.Context() == nil {
		b.Errorf("Context() returned nil")
	}
	for b.Loop() {
		_ = 3 + 3
	}
}
