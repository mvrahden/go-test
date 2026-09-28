// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestspec_test //nolint:fail-guard

import (
	"fmt"
	"runtime"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchScanTestSuite covers how a benchmark node finds its result line in
// output that arrives in arbitrary chunks.
// Sequential: one method measures what the process allocates.
type BenchScanTestSuite struct{}

func benchOutput(chunks ...string) []gotestspec.TestEvent {
	events := make([]gotestspec.TestEvent, 0, len(chunks))
	for _, c := range chunks {
		events = append(events, gotestspec.TestEvent{Action: gotestspec.ActionOutput, Package: "p", Test: "BenchmarkFoo", Output: c})
	}
	return events
}

func scannedLeaf(t *gotest.T, events []gotestspec.TestEvent) *gotestspec.Node {
	pkgs := gotestspec.BuildTree(events)
	mustLen(t, "packages", pkgs, 1)
	mustLen(t, "root nodes", pkgs[0].Nodes, 1)
	return pkgs[0].Nodes[0]
}

func (s *BenchScanTestSuite) TestResultLine(t *gotest.T) {
	t.It("is read whole when it arrives split behind ns/op", func(it *gotest.T) {
		leaf := scannedLeaf(it, benchOutput("BenchmarkFoo-8 \t 1201 \t 985.2 ns/op", "\t 24 B/op \t 3 allocs/op\n"))
		mustEq(it, "iterations", leaf.Iterations, 1201)
		mustEq(it, "B/op", leaf.BytesPerOp, int64(24))
		mustEq(it, "allocs/op", leaf.AllocsPerOp, int64(3))
	})
	t.It("is read without a closing newline", func(it *gotest.T) {
		leaf := scannedLeaf(it, benchOutput("goos: linux\n", "BenchmarkFoo-8 \t 1201 \t 985.2 ns/op"))
		mustEq(it, "status", leaf.Status, gotestspec.StatusPass)
		mustEq(it, "iterations", leaf.Iterations, 1201)
	})
	t.It("is the first one the output carries", func(it *gotest.T) {
		leaf := scannedLeaf(it, benchOutput("BenchmarkFoo-8 \t 10 \t 1.5 ns/op\n", "BenchmarkFoo-8 \t 20 \t 2.5 ns/op\n"))
		mustEq(it, "iterations", leaf.Iterations, 10)
		mustInDelta(it, "ns/op", 1.5, leaf.NsPerOp, 0.001)
	})
}

// A benchmark that logs is scanned as its output arrives. Scanning what was
// already scanned on every event made the cost quadratic: 20k lines took 38s.
// Four times the output must cost about four times as much, not sixteen.
func (s *BenchScanTestSuite) TestCostGrowsWithTheOutput(t *gotest.T) {
	const lines = 1000
	small := allocatedBuilding(t, lines)
	large := allocatedBuilding(t, 4*lines)
	if large > 8*small {
		t.Errorf("%d output lines allocated %d KB, %d lines %d KB: the cost grows faster than the output",
			lines, small>>10, 4*lines, large>>10)
	}
}

// allocatedBuilding returns the bytes BuildTree allocates for a benchmark
// that logs the given number of lines ahead of its result.
func allocatedBuilding(t *gotest.T, lines int) uint64 {
	chunks := make([]string, 0, lines+1)
	for i := range lines {
		chunks = append(chunks, fmt.Sprintf("    bench_test.go:12: warming the cache, round %d\n", i))
	}
	chunks = append(chunks, "BenchmarkFoo-8 \t 1201 \t 985.2 ns/op\n")
	events := benchOutput(chunks...)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	leaf := scannedLeaf(t, events)
	runtime.ReadMemStats(&after)

	mustEq(t, "iterations", leaf.Iterations, 1201)
	return after.TotalAlloc - before.TotalAlloc
}
