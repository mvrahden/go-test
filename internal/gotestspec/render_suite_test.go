package gotestspec_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

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
	for _, want := range []string{"UserService", "Create", "✓ returns ok"} {
		gotest.Contains(t, out, want)
	}
	gotest.NotContains(t, out, "8ms", "a duration shows from one second up")
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

	gotest.Contains(t, out, "=== a/pkg — 1 passed ===")
	gotest.Contains(t, out, "=== b/pkg — 1 passed ===")
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
		"#### UserService — 1 passed\n",
		"\n- **Create**\n",
		"\n  - PASS returns ok\n",
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

// A specification of a run is also its record: what failed outside any
// behavior must not read as absent.
func (s *RenderTestSuite) TestRenderMarkdown_FailuresOutsideBehaviors(t *gotest.T, _ *renderCtx) {
	var buf bytes.Buffer
	gotestspec.RenderMarkdown(&buf, []*gotestspec.Package{
		{
			Path:   "example.com/pkg",
			Status: gotestspec.StatusFail,
			Nodes: []*gotestspec.Node{{
				Kind: gotestspec.KindSuite, Display: "Orders", Status: gotestspec.StatusFail,
				Output: []string{"    TestOrdersTestSuite never ran: shared fixture DBSharedFixture did not come up\n"},
			}},
		},
		{
			Path:   "shared fixtures",
			Status: gotestspec.StatusFail,
			Output: []string{"FAIL: shared fixture setup failed\n", "DBSharedFixture.BeforeAll failed after 1 attempt(s): refused\n"},
		},
	})
	out := buf.String()

	t.It("marks a suite that failed without a behavior, with its reason", func(it *gotest.T) {
		gotest.Contains(it, out, "#### Orders — FAILED\n\n```text\nTestOrdersTestSuite never ran: shared fixture DBSharedFixture did not come up\n```\n")
	})
	t.It("lists a package that failed on its own, with its output", func(it *gotest.T) {
		gotest.Contains(it, out, "### shared fixtures — FAILED\n\n```text\nFAIL: shared fixture setup failed\nDBSharedFixture.BeforeAll failed after 1 attempt(s): refused\n```\n")
	})
	t.It("counts the failed package", func(it *gotest.T) {
		gotest.Contains(it, out, ": 1 failed, 1 failed package.\n")
	})
	t.It("adds nothing to a specification read from source", func(it *gotest.T) {
		var static bytes.Buffer
		gotestspec.RenderMarkdown(&static, userServiceTree(), gotestspec.WithoutVerdicts())
		gotest.NotContains(it, static.String(), "FAILED")
	})
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

// Below four samples a side there is no significance test, only a ±20%
// rule of thumb; the row says so, so nobody reads the mark as a verdict.
func (s *RenderTestSuite) TestRenderTerminal_WithBenchDeltas_FewSamplesAreMarked(t *gotest.T, _ *renderCtx) {
	deltas := []gotestspec.BenchDelta{
		{Key: "p Foo/BenchmarkFoo", OldNs: 100, NewNs: 125, PercentChange: 25, Significant: true, InsufficientSample: true},
		{Key: "p Foo/BenchmarkBar", OldNs: 100, NewNs: 101, PercentChange: 1, InsufficientSample: true},
	}
	out := renderTerminalWith(topLevelBenchmark(), gotestspec.WithBenchDeltas(deltas))

	gotest.Contains(t, out, "p Foo/BenchmarkFoo  100.0  125.0  +25.0% ⚠ (n<4, ±20% heuristic)", "a heuristic regression row")
	gotest.Contains(t, out, "p Foo/BenchmarkBar  100.0  101.0  +1.0% (n<4, ±20% heuristic)", "a heuristic row")
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

// outlineStream is a package with a suite of both test packages: the
// internal one passes, the external one fails a behavior, runs Ping for 1.2s
// and starts DeleteUser without reporting its verdict.
const outlineStream = `{"Time":"2026-11-09T18:02:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite"}
{"Time":"2026-11-09T18:02:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser"}
{"Time":"2026-11-09T18:02:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser/when_a_user_is_created"}
{"Time":"2026-11-09T18:02:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser/when_a_user_is_created/stores_the_email"}
{"Time":"2026-11-09T18:02:00.01Z","Action":"pass","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser/when_a_user_is_created/stores_the_email","Elapsed":0.01}
{"Time":"2026-11-09T18:02:00.01Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser/when_a_user_is_created/has_one_entry"}
{"Time":"2026-11-09T18:02:00.02Z","Action":"pass","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser/when_a_user_is_created/has_one_entry","Elapsed":0.01}
{"Time":"2026-11-09T18:02:00.02Z","Action":"pass","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser/when_a_user_is_created","Elapsed":0.02}
{"Time":"2026-11-09T18:02:00.02Z","Action":"pass","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser","Elapsed":0.02}
{"Time":"2026-11-09T18:02:00.02Z","Action":"pass","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite","Elapsed":0.02}
{"Time":"2026-11-09T18:20:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite"}
{"Time":"2026-11-09T18:20:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01"}
{"Time":"2026-11-09T18:20:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created"}
{"Time":"2026-11-09T18:20:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created/stores_the_email"}
{"Time":"2026-11-09T18:20:00.01Z","Action":"pass","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created/stores_the_email","Elapsed":0.01}
{"Time":"2026-11-09T18:20:00.01Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created/has_one_entry"}
{"Time":"2026-11-09T18:20:00.02Z","Action":"output","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created/has_one_entry","Output":"    suite_ext_test.go:28: Len: expected 1, got 2\n"}
{"Time":"2026-11-09T18:20:00.02Z","Action":"output","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created/has_one_entry","Output":"--- FAIL: TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created/has_one_entry (0.01s)\n"}
{"Time":"2026-11-09T18:20:00.02Z","Action":"fail","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created/has_one_entry","Elapsed":0.01}
{"Time":"2026-11-09T18:20:00.02Z","Action":"fail","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01/when_a_user_is_created","Elapsed":0.02}
{"Time":"2026-11-09T18:20:00.02Z","Action":"fail","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestCreateUser#01","Elapsed":0.02}
{"Time":"2026-11-09T18:20:00.02Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestPing"}
{"Time":"2026-11-09T18:20:01.22Z","Action":"pass","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestPing","Elapsed":1.2}
{"Time":"2026-11-09T18:25:00Z","Action":"run","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite/TestDeleteUser"}
{"Time":"2026-11-09T18:25:30Z","Action":"fail","Package":"github.com/mvrahden/go-test/examples/repository","Test":"TestUserRepositoryTestSuite","Elapsed":330}
{"Time":"2026-11-09T18:25:30Z","Action":"fail","Package":"github.com/mvrahden/go-test/examples/repository","Elapsed":1410}
`

// outlineWant is outlineStream's Markdown spec in a checkout of main with
// uncommitted changes.
const outlineWant = "# Behavior Specification\n" +
	"\n" +
	"2 suites, 6 behaviors: 4 passed, 1 failed, 1 not run.\n" +
	"\n" +
	"Runs from 2026-11-09 18:02 to 18:25 UTC · main @ c94f5cd5, uncommitted changes\n" +
	"\n" +
	"## Needs attention\n" +
	"\n" +
	"- **FAIL** examples/repository › UserRepository › CreateUser › when a user is created › has one entry\n" +
	"  ```text\n" +
	"  suite_ext_test.go:28: Len: expected 1, got 2\n" +
	"  ```\n" +
	"- **NOT RUN** examples/repository › UserRepository › DeleteUser\n" +
	"\n" +
	"## github.com/mvrahden/go-test\n" +
	"\n" +
	"### examples/repository — 4 passed, 1 failed, 1 not run\n" +
	"\n" +
	"#### UserRepository — 2 passed\n" +
	"\n" +
	"- **CreateUser**\n" +
	"  - when a user is created\n" +
	"    - PASS stores the email\n" +
	"    - PASS has one entry\n" +
	"\n" +
	"#### UserRepository — EXTERNAL — 2 passed, 1 failed, 1 not run\n" +
	"\n" +
	"- **CreateUser**\n" +
	"  - when a user is created\n" +
	"    - PASS stores the email\n" +
	"    - **FAIL** has one entry\n" +
	"      ```text\n" +
	"      suite_ext_test.go:28: Len: expected 1, got 2\n" +
	"      ```\n" +
	"- PASS **Ping** (1.2s)\n" +
	"- **NOT RUN** **DeleteUser**\n"

func (s *RenderTestSuite) TestTheOutlineMatchesTheContract(t *gotest.T, _ *renderCtx) {
	out := renderMarkdownOf(treeOf(t, outlineStream),
		gotestspec.WithModule("github.com/mvrahden/go-test"),
		gotestspec.WithProvenance(gotestspec.Provenance{Branch: "main", Commit: "c94f5cd5", Dirty: true}))
	gotest.Equal(t, outlineWant, out)
}

// countsStream starts a behavior that reports no verdict beside one that
// passes.
const countsStream = `{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/passes"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/passes","Elapsed":0.01}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/hangs"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM","Elapsed":2}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":2}
{"Action":"pass","Package":"p","Elapsed":2}
`

func (s *RenderTestSuite) TestCountsAddUp(t *gotest.T, _ *renderCtx) {
	tree := treeOf(t, countsStream)
	stats := gotestspec.CollectStats(tree)
	t.When("a behavior started and reported no verdict", func(w *gotest.T) {
		w.It("counts as not run", func(it *gotest.T) {
			gotest.Equal(it, 1, stats.NotRun)
		})
		w.It("adds up to the behaviors", func(it *gotest.T) {
			gotest.Equal(it, stats.Behaviors, stats.Passed+stats.Failed+stats.Skipped+stats.NotRun)
		})
		w.It("reads NOT RUN with no duration", func(it *gotest.T) {
			gotest.Contains(it, renderMarkdownOf(tree), "\n  - **NOT RUN** hangs\n")
		})
	})
}

func (s *RenderTestSuite) TestDurationsUnderASecondAreLeftOut(t *gotest.T, _ *renderCtx) {
	out := renderMarkdownOf(treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestQuick"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestQuick","Elapsed":0.3}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0.3}
{"Action":"pass","Package":"p","Elapsed":0.3}
`))
	gotest.Contains(t, out, "- PASS **Quick**\n")
}

func (s *RenderTestSuite) TestAMethodNamedWithAnUnderscoreReadsWithout(t *gotest.T, _ *renderCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/Test_HelloWorld"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/Test_HelloWorld","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0}
`)
	gotest.Len(t, tree, 1)
	gotest.Len(t, tree[0].Nodes, 1)
	gotest.Len(t, tree[0].Nodes[0].Children, 1)
	gotest.Equal(t, "HelloWorld", tree[0].Nodes[0].Children[0].Display)
}

// repositoryTree is one package with a passing and a failing suite and a
// method that did not run.
func repositoryTree() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "example.com/mod/examples/repository",
		Nodes: []*gotestspec.Node{{
			Kind:     gotestspec.KindSuite,
			Display:  "UserRepository",
			External: true,
			Children: []*gotestspec.Node{{
				Kind:    gotestspec.KindMethod,
				Display: "Create",
				Children: []*gotestspec.Node{
					{Kind: gotestspec.KindBlock, Display: "stores the email", Status: gotestspec.StatusPass, Duration: 1200 * time.Millisecond},
					{Kind: gotestspec.KindBlock, Display: "rejects a duplicate", Status: gotestspec.StatusPass, Duration: 8 * time.Millisecond},
					{Kind: gotestspec.KindBlock, Display: "hashes the password", Status: gotestspec.StatusFail, Output: []string{"    want 1, got 2\n"}},
				},
			}},
		}},
	}}
}

func (s *RenderTestSuite) TestWrapLabel(t *gotest.T, _ *renderCtx) {
	t.It("breaks at spaces to fit the width", func(it *gotest.T) {
		gotest.Equal(it, []string{"a long", "label that", "wraps"}, gotestspec.ExportWrapLabel("a long label that wraps", 4, 6, 16))
	})
	t.It("keeps a word longer than the width whole on its own line", func(it *gotest.T) {
		gotest.Equal(it, []string{"see", "extraordinarily", "long"}, gotestspec.ExportWrapLabel("see extraordinarily long", 2, 2, 10))
	})
	t.It("does not wrap at width 0", func(it *gotest.T) {
		gotest.Equal(it, []string{"a long label that wraps"}, gotestspec.ExportWrapLabel("a long label that wraps", 4, 6, 0))
	})
	t.It("counts a rune as one column", func(it *gotest.T) {
		gotest.Equal(it, []string{"ääää ääää", "ääää"}, gotestspec.ExportWrapLabel("ääää ääää ääää", 0, 0, 9))
	})
}

func (s *RenderTestSuite) TestTheTerminalListsFailuresLast(t *gotest.T, _ *renderCtx) {
	pkgs := append(repositoryTree(), &gotestspec.Package{
		Path:  "example.com/mod/examples/zeta",
		Nodes: []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "TestZeta", Status: gotestspec.StatusPass}},
	})
	out := renderTerminalWith(pkgs, gotestspec.WithModule("example.com/mod"))

	lastPkg := strings.Index(out, "=== examples/zeta")
	attention := strings.Index(out, "Needs attention")
	summary := strings.LastIndex(out, "suite")
	gotest.Greater(t, lastPkg, -1)
	gotest.Greater(t, attention, lastPkg)
	gotest.Greater(t, summary, attention)
	gotest.Contains(t, out[attention:], "✗ examples/repository › UserRepository › Create › hashes the password\n  want 1, got 2")
}

func (s *RenderTestSuite) TestTheTerminalCarriesCountsOnEveryPackageAndSuite(t *gotest.T, _ *renderCtx) {
	out := renderTerminalWith(repositoryTree(), gotestspec.WithModule("example.com/mod"))
	gotest.Contains(t, out, "=== examples/repository — 2 passed, 1 failed ===")
	gotest.Contains(t, out, "UserRepository — EXTERNAL — 2 passed, 1 failed")
	gotest.Contains(t, out, "✓ stores the email (1.2s)")
	gotest.Contains(t, out, "✓ rejects a duplicate\n")
	gotest.Contains(t, out, "✗ hashes the password\n")
}

func (s *RenderTestSuite) TestTheTerminalWrapsAtItsWidth(t *gotest.T, _ *renderCtx) {
	pkgs := []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind: gotestspec.KindSuite, Display: "S",
			Children: []*gotestspec.Node{{Kind: gotestspec.KindBlock, Display: "a long label that wraps over lines", Status: gotestspec.StatusPass}},
		}},
	}}
	t.It("breaks a long label under its text", func(it *gotest.T) {
		out := renderTerminalWith(pkgs, gotestspec.WithWidth(24))
		gotest.Contains(it, out, "  ✓ a long label that\n    wraps over lines\n")
	})
	t.It("leaves it whole at width 0", func(it *gotest.T) {
		out := renderTerminalWith(pkgs, gotestspec.WithWidth(0))
		gotest.Contains(it, out, "✓ a long label that wraps over lines\n")
	})
}

func (s *RenderTestSuite) TestTheTerminalKeepsATailWithinItsWidth(t *gotest.T, _ *renderCtx) {
	pkgs := []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind: gotestspec.KindSuite, Display: "LongSuiteName", External: true,
			Children: []*gotestspec.Node{
				{Kind: gotestspec.KindBlock, Display: "ok", Status: gotestspec.StatusPass},
				{Kind: gotestspec.KindBlock, Display: "bad", Status: gotestspec.StatusFail},
				{Kind: gotestspec.KindBlock, Display: "later", Status: gotestspec.StatusNone, NotRun: true},
			},
		}},
	}}
	out := renderTerminalWith(pkgs, gotestspec.WithWidth(30))
	rows, _, _ := strings.Cut(out, "Needs attention") // its path and the summary are not rows
	for _, line := range strings.Split(rows, "\n") {
		gotest.LessOrEqual(t, utf8.RuneCountInString(line), 30, "line %q", line)
	}
	gotest.Contains(t, out, "LongSuiteName\n")
	gotest.Contains(t, out, "— EXTERNAL")
}

const outlinePkg = "github.com/mvrahden/go-test/examples/repository"

func (s *RenderTestSuite) TestMatchKeepsTheLeavesHoldingTheTextAndTheirGroups(t *gotest.T, _ *renderCtx) {
	out := renderMarkdownOf(treeOf(t, outlineStream),
		gotestspec.WithModule("github.com/mvrahden/go-test"), gotestspec.WithMatch("EMAIL"))
	gotest.Contains(t, out, "- PASS stores the email\n")
	gotest.Contains(t, out, "- **CreateUser**\n  - when a user is created\n")
	gotest.NotContains(t, out, "has one entry")
	gotest.NotContains(t, out, "Ping")
	gotest.Contains(t, out, "2 suites, 2 behaviors: 2 passed.\n")
	gotest.Contains(t, out, `search "EMAIL"`)
}

func (s *RenderTestSuite) TestMatchWithoutAHitLeavesNoRow(t *gotest.T, _ *renderCtx) {
	var buf bytes.Buffer
	gotestspec.RenderTerminal(&buf, treeOf(t, outlineStream), gotestspec.WithMatch("nothing like it"))
	gotest.NotContains(t, buf.String(), "CreateUser")
}

func (s *RenderTestSuite) TestOnlyRendersOneNodeAndItsSubtree(t *gotest.T, _ *renderCtx) {
	tree := treeOf(t, outlineStream)
	var out bytes.Buffer
	gotestspec.RenderJSON(&out, tree, gotestspec.WithOnly(outlinePkg, []int{1, 1}))
	gotest.Contains(t, out.String(), `"display":"Ping"`)
	gotest.NotContains(t, out.String(), "CreateUser")
	gotest.NotContains(t, out.String(), "DeleteUser")
	md := renderMarkdownOf(tree, gotestspec.WithModule("github.com/mvrahden/go-test"),
		gotestspec.WithOnly(outlinePkg, []int{1, 0, 0}))
	gotest.Contains(t, md, "- when a user is created\n")
	gotest.Contains(t, md, "**FAIL** has one entry")
	gotest.NotContains(t, md, "Ping")
	gotest.Contains(t, md, "only "+outlinePkg+":1.0.0")
	gotest.Contains(t, md, "2 behaviors: 1 passed, 1 failed")
}

func (s *RenderTestSuite) TestOnlyAPackageRendersItWhole(t *gotest.T, _ *renderCtx) {
	md := renderMarkdownOf(treeOf(t, outlineStream), gotestspec.WithOnly(outlinePkg, nil))
	gotest.Contains(t, md, "Ping")
	gotest.Contains(t, md, "only "+outlinePkg+"\n")
}

func (s *RenderTestSuite) TestOnlyNamesWhatIsNotThere(t *gotest.T, _ *renderCtx) {
	tree := treeOf(t, outlineStream)
	gotest.ErrorContains(t, gotestspec.CheckView(tree, gotestspec.WithOnly(outlinePkg, []int{1, 7})),
		"--only: no node at 1.7 in "+outlinePkg)
	gotest.ErrorContains(t, gotestspec.CheckView(tree, gotestspec.WithOnly("nope", nil)), "no package nope")
	gotest.NoError(t, gotestspec.CheckView(tree, gotestspec.WithOnly(outlinePkg, []int{1, 1})))
}

func (s *RenderTestSuite) TestOnlyPositionsAreThoseOfTheShownRows(t *gotest.T, _ *renderCtx) {
	tree := treeOf(t, outlineStream)
	hide := gotestspec.WithHidden(gotestspec.StatusPass)
	var doc bytes.Buffer
	gotestspec.RenderJSON(&doc, tree, hide)
	var got struct {
		Packages []struct {
			Nodes []struct {
				Display  string
				Children []struct{ Display string }
			}
		}
	}
	gotest.NoError(t, json.Unmarshal(doc.Bytes(), &got))
	want := got.Packages[0].Nodes[0].Children[1].Display
	var md bytes.Buffer
	gotestspec.RenderJSON(&md, tree, hide, gotestspec.WithOnly(outlinePkg, []int{0, 1}))
	gotest.Contains(t, md.String(), `"display":"`+want+`"`)
}

func (s *RenderTestSuite) TestMatchReadsTheRowsHideLeavesShown(t *gotest.T, _ *renderCtx) {
	tree := []*gotestspec.Package{{
		Path: "example.com/pkg",
		Nodes: []*gotestspec.Node{{
			Kind: gotestspec.KindSuite, Display: "Cart", Status: gotestspec.StatusFail,
			Children: []*gotestspec.Node{{
				Kind: gotestspec.KindBlock, Display: "totals", Status: gotestspec.StatusFail,
				Children: []*gotestspec.Node{{Kind: gotestspec.KindBlock, Display: "adds up", Status: gotestspec.StatusPass}},
			}},
		}},
	}}
	hide := gotestspec.WithHidden(gotestspec.StatusPass)
	// The failing group has only a passing child: hidden, the group is a leaf.
	var kept bytes.Buffer
	gotestspec.RenderJSON(&kept, tree, hide, gotestspec.WithMatch("TOTALS"))
	gotest.Contains(t, kept.String(), `"display":"totals"`)
	// A hidden leaf that matches does not keep its group.
	var dropped bytes.Buffer
	gotestspec.RenderJSON(&dropped, tree, hide, gotestspec.WithMatch("adds up"))
	gotest.NotContains(t, dropped.String(), `"display":"totals"`)
}
