// Package benchzero holds a benchmark every sample of which measured no time,
// as a coarse clock can measure an empty loop.
package benchzero

import "github.com/mvrahden/go-test/pkg/gotest"

type ZeroTestSuite struct{}

// BenchmarkIdle reports 0 ns/op, which go test prints as it prints a sample
// that measured nothing: without ns/op.
func (s *ZeroTestSuite) BenchmarkIdle(b *gotest.B) {
	for b.Loop() {
	}
	b.ReportMetric(0, "ns/op")
}
