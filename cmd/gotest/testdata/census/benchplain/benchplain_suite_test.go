package benchplain_test

import "github.com/mvrahden/go-test/pkg/gotest"

// BenchPlainTestSuite declares two benchmarks and one test; a bench run must
// produce a result for both benchmarks before it is believed.
type BenchPlainTestSuite struct{}

func (s *BenchPlainTestSuite) TestOne(t *gotest.T) { gotest.True(t, true) }

func (s *BenchPlainTestSuite) BenchmarkA(b *gotest.B) {
	for b.Loop() {
		_ = 1 * 1
	}
}

func (s *BenchPlainTestSuite) BenchmarkB(b *gotest.B) {
	for b.Loop() {
		_ = 1 + 1
	}
}
