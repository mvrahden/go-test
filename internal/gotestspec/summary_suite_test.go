package gotestspec_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SummaryRenderTestSuite covers the failure-focused summary in terminal and
// markdown form: what a green run prints, how failures are located and
// filtered, and how package-level diagnostics surface.
type SummaryRenderTestSuite struct{}

func (s *SummaryRenderTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type summaryRenderCtx struct{}

func (s *SummaryRenderTestSuite) BeforeEach(t *gotest.T) *summaryRenderCtx {
	return &summaryRenderCtx{}
}

func renderSummary(packages []*gotestspec.Package) string {
	var buf bytes.Buffer
	gotestspec.RenderSummary(&buf, packages, gotestspec.WithNoColor())
	return buf.String()
}

func renderMarkdownSummary(packages []*gotestspec.Package) string {
	var buf bytes.Buffer
	gotestspec.RenderMarkdownSummary(&buf, packages)
	return buf.String()
}

func failingTest(display, output string) *gotestspec.Node {
	return &gotestspec.Node{
		Kind:     gotestspec.KindTest,
		Display:  display,
		Status:   gotestspec.StatusFail,
		Duration: time.Millisecond,
		Output:   []string{output},
	}
}

func (s *SummaryRenderTestSuite) TestRenderSummary_AllPass(t *gotest.T, _ *summaryRenderCtx) {
	out := renderSummary([]*gotestspec.Package{{
		Path:     "example.com/pkg",
		Duration: 2300 * time.Millisecond,
		Nodes: []*gotestspec.Node{
			{Kind: gotestspec.KindTest, Display: "Foo", Status: gotestspec.StatusPass, Duration: time.Second},
			{Kind: gotestspec.KindTest, Display: "Bar", Status: gotestspec.StatusPass, Duration: time.Second},
		},
	}})

	gotest.Contains(t, out, "2 tests passed")
	gotest.NotContains(t, out, "FAIL")
}

func (s *SummaryRenderTestSuite) TestRenderSummary_WithFailures(t *gotest.T, _ *summaryRenderCtx) {
	out := renderSummary([]*gotestspec.Package{{
		Path: "example.com/pkg/foo",
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "UserService",
			Children: []*gotestspec.Node{
				{
					Kind:    gotestspec.KindMethod,
					Display: "Create",
					Children: []*gotestspec.Node{
						{Kind: gotestspec.KindBlock, Display: "returns ok", Status: gotestspec.StatusPass, Duration: 5 * time.Millisecond},
						{
							Kind:     gotestspec.KindBlock,
							Display:  "rejects empty name",
							Status:   gotestspec.StatusFail,
							Duration: 12 * time.Millisecond,
							Output:   []string{"    user_test.go:42: expected error, got nil\n"},
						},
					},
				},
				{Kind: gotestspec.KindMethod, Display: "Delete", Status: gotestspec.StatusPass, Duration: 3 * time.Millisecond},
			},
		}},
	}})

	gotest.Contains(t, out, "1 of 3 tests failed")
	gotest.Contains(t, out, "FAIL")
	gotest.Contains(t, out, "UserService / Create / rejects empty name")
	gotest.Contains(t, out, "expected error, got nil")
	gotest.NotContains(t, out, "returns ok", "passing tests do not appear in the summary")
}

func (s *SummaryRenderTestSuite) TestRenderSummary_FiltersNoise(t *gotest.T, _ *summaryRenderCtx) {
	out := renderSummary([]*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:     gotestspec.KindTest,
			Display:  "Broken",
			Status:   gotestspec.StatusFail,
			Duration: time.Millisecond,
			Output: []string{
				"=== RUN   TestBroken\n",
				"--- FAIL: TestBroken (0.00s)\n",
				"    broken_test.go:10: assertion failed\n",
			},
		}},
	}})

	gotest.Contains(t, out, "assertion failed")
	gotest.NotContains(t, out, "=== RUN")
	gotest.NotContains(t, out, "--- FAIL")
}

func (s *SummaryRenderTestSuite) TestRenderSummary_MultiplePackages(t *gotest.T, _ *summaryRenderCtx) {
	out := renderSummary([]*gotestspec.Package{
		{Path: "pkg/a", Nodes: []*gotestspec.Node{failingTest("TestA", "    a_test.go:1: boom\n")}},
		{Path: "pkg/b", Nodes: []*gotestspec.Node{failingTest("TestB", "    b_test.go:2: crash\n")}},
	})

	gotest.Contains(t, out, "pkg/a")
	gotest.Contains(t, out, "pkg/b")
}

func (s *SummaryRenderTestSuite) TestRenderMarkdownSummary_AllPass(t *gotest.T, _ *summaryRenderCtx) {
	out := renderMarkdownSummary([]*gotestspec.Package{{
		Path:     "p",
		Duration: 500 * time.Millisecond,
		Nodes: []*gotestspec.Node{
			{Kind: gotestspec.KindTest, Display: "A", Status: gotestspec.StatusPass},
			{Kind: gotestspec.KindTest, Display: "B", Status: gotestspec.StatusPass},
		},
	}})

	gotest.Contains(t, out, "All 2 tests passed")
}

func (s *SummaryRenderTestSuite) TestRenderMarkdownSummary_WithFailures(t *gotest.T, _ *summaryRenderCtx) {
	out := renderMarkdownSummary([]*gotestspec.Package{{
		Path: "pkg/foo",
		Nodes: []*gotestspec.Node{
			{Kind: gotestspec.KindTest, Display: "Good", Status: gotestspec.StatusPass},
			{
				Kind:     gotestspec.KindTest,
				Display:  "Bad",
				Status:   gotestspec.StatusFail,
				Duration: 100 * time.Millisecond,
				Output:   []string{"    foo_test.go:10: want 1, got 2\n"},
			},
		},
	}})

	gotest.Contains(t, out, "1 of 2 tests failed")
	gotest.Contains(t, out, "<details>")
	gotest.Contains(t, out, "pkg/foo")
	gotest.Contains(t, out, "want 1, got 2")
}

func (s *SummaryRenderTestSuite) TestCollectFailures_DeepHierarchy(t *gotest.T, _ *summaryRenderCtx) {
	failures := gotestspec.ExportCollectFailures([]*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "Suite",
			Children: []*gotestspec.Node{{
				Kind:    gotestspec.KindMethod,
				Display: "Method",
				Children: []*gotestspec.Node{
					{
						Kind:    gotestspec.KindBlock,
						Display: "when valid",
						Children: []*gotestspec.Node{
							{Kind: gotestspec.KindBlock, Display: "returns ok", Status: gotestspec.StatusPass},
							{Kind: gotestspec.KindBlock, Display: "logs event", Status: gotestspec.StatusFail, Output: []string{"err"}},
						},
					},
					{Kind: gotestspec.KindBlock, Display: "when invalid", Status: gotestspec.StatusPass},
				},
			}},
		}},
	}})

	gotest.Len(t, failures, 1)
	gotest.Equal(t, "Suite / Method / when valid / logs event", strings.Join(failures[0].Display, " / "))
}

var raceDiagnostic = []string{
	"==================\n",
	"WARNING: DATA RACE\n",
	"Write at 0x00c by goroutine 9:\n",
	"  pkg.TestFoo.func1()\n",
	"      foo_test.go:12 +0x38\n",
	"==================\n",
	"Found 1 data race(s)\n",
}

func (s *SummaryRenderTestSuite) TestRenderSummary_PackageDiagnostic(t *gotest.T, _ *summaryRenderCtx) {
	out := renderSummary([]*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes:  []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "Foo", Status: gotestspec.StatusPass, Duration: time.Millisecond}},
		Output: raceDiagnostic,
	}})

	gotest.Contains(t, out, "WARNING: DATA RACE")
	gotest.Contains(t, out, "example.com/pkg")
	gotest.Contains(t, out, "package failure detected")
}

func (s *SummaryRenderTestSuite) TestRenderMarkdownSummary_PackageDiagnostic(t *gotest.T, _ *summaryRenderCtx) {
	out := renderMarkdownSummary([]*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes:  []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "Foo", Status: gotestspec.StatusPass, Duration: time.Millisecond}},
		Output: []string{"==================\n", "WARNING: DATA RACE\n", "Found 1 data race(s)\n"},
	}})

	gotest.Contains(t, out, "WARNING: DATA RACE")
	gotest.Contains(t, out, "<details>")
}

func (s *SummaryRenderTestSuite) TestRenderSummary_BothTestFailureAndPackageDiagnostic(t *gotest.T, _ *summaryRenderCtx) {
	out := renderSummary([]*gotestspec.Package{{
		Path:   "p",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{
			{Kind: gotestspec.KindTest, Display: "Good", Status: gotestspec.StatusPass, Duration: time.Millisecond},
			failingTest("Bad", "    foo_test.go:10: assertion failed\n"),
		},
		Output: []string{"WARNING: DATA RACE\n", "Found 1 data race(s)\n"},
	}})

	gotest.Contains(t, out, "assertion failed")
	gotest.Contains(t, out, "WARNING: DATA RACE")
}

func singlePassingTest() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path:  "p",
		Nodes: []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "Foo", Status: gotestspec.StatusPass, Duration: time.Millisecond}},
	}}
}

func (s *SummaryRenderTestSuite) TestRenderSummary_WithBenchDeltas(t *gotest.T, _ *summaryRenderCtx) {
	deltas := []gotestspec.BenchDelta{
		{Key: "p Foo/BenchmarkFoo", OldNs: 100, NewNs: 150, PercentChange: 50, Significant: true},
		{Key: "p Foo/BenchmarkBar", OldNs: 100, NewNs: 105, PercentChange: 5, Significant: false},
	}
	var buf bytes.Buffer
	gotestspec.RenderSummary(&buf, singlePassingTest(), gotestspec.WithNoColor(), gotestspec.WithBenchDeltas(deltas))
	out := buf.String()

	gotest.Contains(t, out, "BENCHMARK  OLD ns/op  NEW ns/op  Δ", "delta table header")
	gotest.Contains(t, out, "p Foo/BenchmarkFoo  100.0  150.0  +50.0% ⚠", "regression row with warning")
	gotest.Contains(t, out, "p Foo/BenchmarkBar  100.0  105.0  +5.0%\n", "insignificant row without warning")
	gotest.NotContains(t, out, "p Foo/BenchmarkBar  100.0  105.0  +5.0% ⚠", "insignificant row carries no warning")
}

func (s *SummaryRenderTestSuite) TestRenderSummary_NoBenchDeltas(t *gotest.T, _ *summaryRenderCtx) {
	gotest.NotContains(t, renderSummary(singlePassingTest()), "BENCHMARK", "no delta table without deltas")
}

func (s *SummaryRenderTestSuite) TestRenderMarkdownSummary_WithBenchDeltas(t *gotest.T, _ *summaryRenderCtx) {
	packages := []*gotestspec.Package{{
		Path:  "pkg/foo",
		Nodes: []*gotestspec.Node{failingTest("Bad", "    foo_test.go:1: boom\n")},
	}}
	deltas := []gotestspec.BenchDelta{
		{Key: "p Foo/BenchmarkFoo", OldNs: 100, NewNs: 200, PercentChange: 100, Significant: true},
		{Key: "p Foo/BenchmarkBar", OldNs: 100, NewNs: 102, PercentChange: 2, Significant: false},
	}
	var buf bytes.Buffer
	gotestspec.RenderMarkdownSummary(&buf, packages, gotestspec.WithBenchDeltas(deltas))
	out := buf.String()

	gotest.Contains(t, out, "| Benchmark | old ns/op | new ns/op | Δ |", "markdown delta table header")
	gotest.Contains(t, out, "| p Foo/BenchmarkFoo | 100.0 | 200.0 | +100.0% ⚠ |", "regression row")
	gotest.Contains(t, out, "| p Foo/BenchmarkBar | 100.0 | 102.0 | +2.0% |", "insignificant row")

	tableIdx := strings.Index(out, "| Benchmark |")
	ruleIdx := strings.Index(out, "---\n")
	gotest.True(t, tableIdx != -1 && ruleIdx != -1 && tableIdx < ruleIdx, "the delta table precedes the trailing ---, got:\n%s", out)
}

func (s *SummaryRenderTestSuite) TestRenderMarkdownSummary_NoBenchDeltas(t *gotest.T, _ *summaryRenderCtx) {
	gotest.NotContains(t, renderMarkdownSummary(singlePassingTest()), "| Benchmark |", "no delta table without deltas")
}
