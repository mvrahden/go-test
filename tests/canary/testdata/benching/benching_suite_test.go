package benching_test

import "github.com/mvrahden/go-test/pkg/gotest"

// BenchingTestSuite declares two benchmarks; a bench run must report both.
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
