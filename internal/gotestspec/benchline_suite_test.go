// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestspec_test //nolint:fail-guard

import (
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchLineTestSuite covers how a benchmark result line is read: the name,
// the iteration count and every "<value> <unit>" pair that follows.
type BenchLineTestSuite struct{}

func (s *BenchLineTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *BenchLineTestSuite) TestResultLine(t *gotest.T) {
	t.It("keeps B/op and allocs/op when a reported metric sits between them and ns/op", func(it *gotest.T) {
		line, ok := gotestspec.ParseBenchLine("BenchmarkFoo-8 \t 1201 \t 985.2 ns/op \t 4.500 widgets/op \t 24 B/op \t 3 allocs/op")
		mustEq(it, "ok", ok, true)
		mustEq(it, "name", line.Name, "BenchmarkFoo")
		mustEq(it, "procs", line.Procs, 8)
		mustEq(it, "iterations", line.Iterations, 1201)
		mustInDelta(it, "ns/op", 985.2, line.NsPerOp, 0.001)
		mustEq(it, "B/op", line.BytesPerOp, int64(24))
		mustEq(it, "allocs/op", line.AllocsPerOp, int64(3))
		mustInDelta(it, "widgets/op", 4.5, line.Metrics["widgets/op"], 0.001)
		mustLen(it, "metrics", mapKeys(line.Metrics), 1)
	})

	t.It("keeps MB/s and a metric with no /op as metrics", func(it *gotest.T) {
		line, ok := gotestspec.ParseBenchLine("BenchmarkFoo/sub-4 \t 10 \t 1.5 ns/op \t 512.00 MB/s \t 7 misses")
		mustEq(it, "ok", ok, true)
		mustEq(it, "name", line.Name, "BenchmarkFoo/sub")
		mustEq(it, "procs", line.Procs, 4)
		mustInDelta(it, "MB/s", 512, line.Metrics["MB/s"], 0.001)
		mustInDelta(it, "misses", 7, line.Metrics["misses"], 0.001)
	})

	t.It("rejects a line without ns/op or without an iteration count", func(it *gotest.T) {
		_, ok := gotestspec.ParseBenchLine("BenchmarkFoo-8 \t 1201 \t 24 B/op")
		mustEq(it, "no ns/op", ok, false)
		_, ok = gotestspec.ParseBenchLine("BenchmarkFoo-8 \t 985.2 ns/op")
		mustEq(it, "no count", ok, false)
		_, ok = gotestspec.ParseBenchLine("goos: linux")
		mustEq(it, "not a result", ok, false)
	})
}

func mapKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
