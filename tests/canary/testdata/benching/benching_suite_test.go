package benching_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchingTestSuite declares green benchmarks of every shape: plain ones,
// one that reports a metric and one that opens sub-benchmarks. A bench run
// must report a result for each leaf, and Context() must be usable inside one.
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

func (s *BenchingTestSuite) BenchmarkMetric(b *gotest.B) {
	for b.Loop() {
		_ = 4 + 4
	}
	b.ReportMetric(4.5, "widgets/op")
}

func (s *BenchingTestSuite) BenchmarkSplit(b *gotest.B) {
	for _, name := range []string{"short", "long"} {
		b.B().Run(name, func(b *testing.B) {
			for b.Loop() {
				_ = len(name)
			}
		})
	}
}
