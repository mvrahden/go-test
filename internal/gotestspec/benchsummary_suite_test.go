package gotestspec_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchSummaryTestSuite tests the markdown a bench run leaves in a GitHub
// job summary: benchmark counts and results, never the test headline.
type BenchSummaryTestSuite struct{}

func (s *BenchSummaryTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func benchLeaf(name string, ns float64, bytesPerOp, allocs int64) *gotestspec.Node {
	return &gotestspec.Node{
		Kind:        gotestspec.KindBenchmark,
		Name:        name,
		Display:     strings.TrimPrefix(name, "Benchmark"),
		Status:      gotestspec.StatusPass,
		Iterations:  10,
		NsPerOp:     ns,
		BytesPerOp:  bytesPerOp,
		AllocsPerOp: allocs,
	}
}

func benchPackages() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path:     "example.com/cache",
		Duration: 12 * time.Millisecond,
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindBenchmark,
			Name:    "BenchmarkCacheTestSuite",
			Display: "BenchmarkCache",
			Status:  gotestspec.StatusPass,
			Children: []*gotestspec.Node{
				benchLeaf("BenchmarkGetHit", 231, 0, 0),
				benchLeaf("BenchmarkPutEviction", 1213.4, 48, 1),
			},
		}},
	}}
}

func renderBench(packages []*gotestspec.Package, opts ...gotestspec.RenderOption) string {
	var buf bytes.Buffer
	gotestspec.RenderMarkdownBenchSummary(&buf, packages, opts...)
	return buf.String()
}

func (s *BenchSummaryTestSuite) TestResults(t *gotest.T) {
	t.When("every benchmark ran", func(w *gotest.T) {
		out := renderBench(benchPackages())

		w.It("headlines the benchmark count and duration", func(it *gotest.T) {
			gotest.Contains(it, out, "### 2 benchmarks ran (12ms)")
			gotest.NotContains(it, out, "tests passed")
		})
		w.It("lists each package's results under its path", func(it *gotest.T) {
			gotest.Contains(it, out, "**example.com/cache**")
			gotest.Contains(it, out, "| Benchmark | ns/op | B/op | allocs/op |")
		})
		w.It("keys rows the way the delta table does", func(it *gotest.T) {
			gotest.Contains(it, out, "| CacheTestSuite/BenchmarkGetHit | 231 | 0 | 0 |")
			gotest.Contains(it, out, "| CacheTestSuite/BenchmarkPutEviction | 1213.4 | 48 | 1 |")
		})
	})

	t.When("the caller measured the run's wall clock", func(w *gotest.T) {
		out := renderBench(benchPackages(), gotestspec.WithElapsed(2300*time.Millisecond))

		w.It("headlines that instead of the packages' own timings", func(it *gotest.T) {
			gotest.Contains(it, out, "### 2 benchmarks ran (2.3s)")
		})
	})

	t.When("a benchmark is a bare top-level function", func(w *gotest.T) {
		packages := []*gotestspec.Package{{
			Path:  "example.com/raw",
			Nodes: []*gotestspec.Node{benchLeaf("BenchmarkParse", 970, 24, 3)},
		}}
		out := renderBench(packages)

		w.It("keys the row by its own name", func(it *gotest.T) {
			gotest.Contains(it, out, "| BenchmarkParse | 970 | 24 | 3 |")
		})
	})
}

func (s *BenchSummaryTestSuite) TestFailure(t *gotest.T) {
	t.When("a benchmark fails", func(w *gotest.T) {
		packages := benchPackages()
		failed := packages[0].Nodes[0].Children[1]
		failed.Status = gotestspec.StatusFail
		failed.Iterations = 0
		failed.Output = []string{"    cache_test.go:9: eviction lost a key\n"}
		packages[0].Nodes[0].Status = gotestspec.StatusFail
		out := renderBench(packages)

		w.It("headlines the failure count", func(it *gotest.T) {
			gotest.Contains(it, out, "### 1 of 2 benchmarks failed (12ms)")
		})
		w.It("quotes the failure output", func(it *gotest.T) {
			gotest.Contains(it, out, "<summary><b>example.com/cache</b> — BenchmarkCache / PutEviction")
			gotest.Contains(it, out, "cache_test.go:9: eviction lost a key")
		})
		w.It("shows no timing for a benchmark that never measured", func(it *gotest.T) {
			gotest.Contains(it, out, "| CacheTestSuite/BenchmarkPutEviction | — | — | — |")
		})
	})
}

func (s *BenchSummaryTestSuite) TestComparison(t *gotest.T) {
	deltas := []gotestspec.BenchDelta{
		{Key: "example.com/cache CacheTestSuite/BenchmarkGetHit", OldNs: 100, NewNs: 142.9, PercentChange: 42.9, Significant: true},
	}

	t.When("a baseline comparison ran", func(w *gotest.T) {
		out := renderBench(benchPackages(), gotestspec.WithBenchDeltas(deltas))

		w.It("renders the delta table after the results", func(it *gotest.T) {
			gotest.Contains(it, out, "| example.com/cache CacheTestSuite/BenchmarkGetHit | 100.0 | 142.9 | +42.9% ⚠ |")
			gotest.Less(it, strings.Index(out, "| ns/op |"), strings.Index(out, "| old ns/op |"))
		})
		w.It("states no gate verdict without a gate", func(it *gotest.T) {
			gotest.NotContains(it, out, "Bench gate")
		})
	})

	t.When("the gate is breached", func(w *gotest.T) {
		gate := &gotestspec.BenchGate{ThresholdPct: 5, Breached: true, WorstKey: "example.com/cache CacheTestSuite/BenchmarkGetHit", WorstPct: 42.9}
		out := renderBench(benchPackages(), gotestspec.WithBenchDeltas(deltas), gotestspec.WithBenchGate(gate))

		w.It("names the worst regression against the threshold", func(it *gotest.T) {
			gotest.Contains(it, out, "**Bench gate breached:** example.com/cache CacheTestSuite/BenchmarkGetHit +42.9% exceeds the 5% gate")
		})
	})

	t.When("the gate holds", func(w *gotest.T) {
		gate := &gotestspec.BenchGate{ThresholdPct: 5, WorstKey: "example.com/cache CacheTestSuite/BenchmarkGetHit", WorstPct: 1.2}
		out := renderBench(benchPackages(), gotestspec.WithBenchDeltas(deltas), gotestspec.WithBenchGate(gate))

		w.It("says so with the threshold", func(it *gotest.T) {
			gotest.Contains(it, out, "**Bench gate passed:** no regression above 5%")
		})
	})
}
