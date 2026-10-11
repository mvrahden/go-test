package benchonly_test

import "github.com/mvrahden/go-test/pkg/gotest"

// BenchOnlyTestSuite declares only a benchmark: a test run builds the package
// and runs nothing.
type BenchOnlyTestSuite struct{}

func (s *BenchOnlyTestSuite) BenchmarkA(b *gotest.B) {
	for b.Loop() {
		_ = 1 + 1
	}
}
