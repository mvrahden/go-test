package testpkg

import "testing"

type StdlibBenchTestSuite struct{}

func (s *StdlibBenchTestSuite) BenchmarkRaw(b *testing.B) {
	for b.Loop() {
	}
}
