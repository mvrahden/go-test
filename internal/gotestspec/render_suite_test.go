package gotestspec_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RenderTestSuite covers the terminal and markdown spec renderers.
type RenderTestSuite struct{}

func (s *RenderTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type renderCtx struct{}

func (s *RenderTestSuite) BeforeEach(t *gotest.T) *renderCtx { return &renderCtx{} }

func userServiceTree() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "example.com/pkg",
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "UserService",
			Children: []*gotestspec.Node{{
				Kind:    gotestspec.KindMethod,
				Display: "Create",
				Children: []*gotestspec.Node{{
					Kind:     gotestspec.KindBlock,
					Display:  "returns ok",
					Status:   gotestspec.StatusPass,
					Duration: 8 * time.Millisecond,
				}},
			}},
		}},
	}}
}

func renderTerminal(packages []*gotestspec.Package) string {
	var buf bytes.Buffer
	gotestspec.RenderTerminal(&buf, packages)
	return gotestspec.ExportStripANSI(buf.String())
}

func (s *RenderTestSuite) TestRenderTerminal_SuiteHierarchy(t *gotest.T, _ *renderCtx) {
	out := renderTerminal(userServiceTree())
	for _, want := range []string{"UserService", "Create", "✓ returns ok", "(8ms)"} {
		gotest.Contains(t, out, want)
	}
}

func (s *RenderTestSuite) TestRenderTerminal_FailedLeaf(t *gotest.T, _ *renderCtx) {
	out := renderTerminal([]*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindTest,
			Display: "Broken",
			Children: []*gotestspec.Node{{
				Kind:     gotestspec.KindBlock,
				Display:  "explodes",
				Status:   gotestspec.StatusFail,
				Duration: 2 * time.Millisecond,
				Output:   []string{"    expected 1, got 2\n"},
			}},
		}},
	}})

	gotest.Contains(t, out, "✗ explodes")
	gotest.Contains(t, out, "expected 1, got 2")
}

func (s *RenderTestSuite) TestRenderTerminal_MultiPackage(t *gotest.T, _ *renderCtx) {
	out := renderTerminal([]*gotestspec.Package{
		{Path: "a/pkg", Nodes: []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "Foo", Status: gotestspec.StatusPass, Duration: time.Millisecond}}},
		{Path: "b/pkg", Nodes: []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "Bar", Status: gotestspec.StatusPass, Duration: time.Millisecond}}},
	})

	gotest.Contains(t, out, "=== a/pkg ===")
	gotest.Contains(t, out, "=== b/pkg ===")
}

func (s *RenderTestSuite) TestRenderTerminal_SummaryLine(t *gotest.T, _ *renderCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc  string
		stats gotestspec.Stats
		want  string
	}{
		{Desc: "suites only", stats: gotestspec.Stats{Suites: 2, Behaviors: 5, Passed: 5}, want: "2 suites, 5 behaviors: 5 passed"},
		{Desc: "stdlib only", stats: gotestspec.Stats{Tests: 3, Passed: 2, Failed: 1}, want: "3 stdlib tests: 2 passed, 1 failed"},
		{Desc: "mixed", stats: gotestspec.Stats{Suites: 1, Behaviors: 2, Tests: 1, Passed: 2, Skipped: 1}, want: "1 suite, 2 behaviors, 1 stdlib test: 2 passed, 1 skipped"},
	}) {
		var buf bytes.Buffer
		gotestspec.ExportRenderSummaryLine(&buf, tc.stats, gotestspec.ExportANSIColors)
		gotest.Equal(sub, tc.want, strings.TrimSpace(gotestspec.ExportStripANSI(buf.String())))
	}
}

func (s *RenderTestSuite) TestRenderMarkdown_SuiteHierarchy(t *gotest.T, _ *renderCtx) {
	var buf bytes.Buffer
	gotestspec.RenderMarkdown(&buf, userServiceTree())
	out := buf.String()

	for _, want := range []string{
		"# Behavior Specification",
		"## UserService",
		"### Create",
		"| returns ok | PASS | 8ms |",
	} {
		gotest.Contains(t, out, want)
	}
}

func (s *RenderTestSuite) TestRenderMarkdown_SkippedSuite(t *gotest.T, _ *renderCtx) {
	var buf bytes.Buffer
	gotestspec.RenderMarkdown(&buf, []*gotestspec.Package{{
		Path:  "p",
		Nodes: []*gotestspec.Node{{Kind: gotestspec.KindSuite, Display: "Broken", Excluded: true, Status: gotestspec.StatusSkip}},
	}})

	gotest.Contains(t, buf.String(), "Broken — SKIPPED")
}

func benchmarkLeaf() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "Foo",
			Children: []*gotestspec.Node{{
				Kind:        gotestspec.KindBenchmark,
				Display:     "Parse",
				Status:      gotestspec.StatusPass,
				Iterations:  1201,
				NsPerOp:     985.2,
				BytesPerOp:  24,
				AllocsPerOp: 3,
			}},
		}},
	}}
}

func topLevelBenchmark() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:       gotestspec.KindBenchmark,
			Display:    "BenchmarkFoo",
			Status:     gotestspec.StatusPass,
			Iterations: 100,
			NsPerOp:    120,
		}},
	}}
}

func renderTerminalWith(packages []*gotestspec.Package, opts ...gotestspec.RenderOption) string {
	var buf bytes.Buffer
	gotestspec.RenderTerminal(&buf, packages, opts...)
	return gotestspec.ExportStripANSI(buf.String())
}

func (s *RenderTestSuite) TestRenderTerminal_BenchmarkLeaf(t *gotest.T, _ *renderCtx) {
	out := renderTerminal(benchmarkLeaf())
	for _, want := range []string{"Parse", "985.2 ns/op", "24 B/op", "3 allocs/op", "1 benchmark"} {
		gotest.Contains(t, out, want)
	}
}

func (s *RenderTestSuite) TestRenderTerminal_WithBenchDeltas(t *gotest.T, _ *renderCtx) {
	deltas := []gotestspec.BenchDelta{
		{Key: "p Foo/BenchmarkFoo", OldNs: 100, NewNs: 150, PercentChange: 50, Significant: true},
	}
	out := renderTerminalWith(topLevelBenchmark(), gotestspec.WithBenchDeltas(deltas))

	gotest.Contains(t, out, "ns/op", "the benchmark result line renders")
	gotest.Contains(t, out, "BENCHMARK  OLD ns/op  NEW ns/op  Δ", "delta table header")
	gotest.Contains(t, out, "p Foo/BenchmarkFoo  100.0  150.0  +50.0% ⚠", "regression row")
	// A bench-only run has counts but no pass/fail verdicts, so the trailer
	// stops at the counts: the colon would have nothing after it.
	gotest.Contains(t, out, "1 benchmark", "the trailing counts line")
	gotest.NotContains(t, out, "tests passed (", "a single summary trailer, not a stacked one")
}

func (s *RenderTestSuite) TestRenderTerminal_WithBenchDeltas_FilteredToEmptyStillPrintsHeader(t *gotest.T, _ *renderCtx) {
	// An empty-but-non-nil slice models what the CLI passes when a
	// comparison ran but every delta was filtered out (no significant
	// regression, -v not passed): the header still proves a comparison
	// happened, beside the tree's own ns/op line.
	out := renderTerminalWith(topLevelBenchmark(), gotestspec.WithBenchDeltas([]gotestspec.BenchDelta{}))

	gotest.Contains(t, out, "ns/op")
	gotest.Contains(t, out, "BENCHMARK  OLD ns/op  NEW ns/op  Δ", "delta table header even with zero rows")
}

func (s *RenderTestSuite) TestRenderTerminal_NoBenchDeltas(t *gotest.T, _ *renderCtx) {
	out := renderTerminal([]*gotestspec.Package{{
		Path:  "p",
		Nodes: []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "Foo", Status: gotestspec.StatusPass, Duration: time.Millisecond}},
	}})
	gotest.NotContains(t, out, "BENCHMARK", "no delta table without WithBenchDeltas")
}

func (s *RenderTestSuite) TestRenderMarkdown_BenchmarkTable(t *gotest.T, _ *renderCtx) {
	var buf bytes.Buffer
	gotestspec.RenderMarkdown(&buf, benchmarkLeaf())
	out := buf.String()

	gotest.Contains(t, out, "| Benchmark | ns/op | B/op | allocs/op |")
	gotest.Contains(t, out, "| Parse | 985.2 | 24 | 3 |")
}
