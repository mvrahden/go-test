package benches_test

import "github.com/mvrahden/go-test/pkg/gotest"

type BenchesTestSuite struct{}

func (s *BenchesTestSuite) BenchmarkLoop(b *gotest.B) {
	for b.B().Loop() {
	}
}
