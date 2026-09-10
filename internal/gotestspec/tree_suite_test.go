// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestspec_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TreeTestSuite covers how a stream becomes the tree: node kinds and labels,
// focus and exclusion, duplicate suites across ptest and pxtest, package-level
// diagnostics, and the stats that count verdicts.
type TreeTestSuite struct{}

func (s *TreeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type treeCtx struct{}

func (s *TreeTestSuite) BeforeEach(t *gotest.T) *treeCtx { return &treeCtx{} }

func (s *TreeTestSuite) TestBuildTree_SuiteHierarchy(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate"}
{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid"}
{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid/creates_the_user"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid/creates_the_user","Elapsed":0.008}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid","Elapsed":0.009}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate","Elapsed":0.01}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite","Elapsed":0.011}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.5}`)
	mustLen(t, "packages", tree, 1)
	pkg := tree[0]
	mustEq(t, "package path", pkg.Path, "example.com/pkg")
	mustLen(t, "root nodes", pkg.Nodes, 1)

	suite := pkg.Nodes[0]
	mustEq(t, "root kind", suite.Kind, gotestspec.KindSuite)
	mustEq(t, "suite display", suite.Display, "UserService")
	mustLen(t, "methods", suite.Children, 1)

	method := suite.Children[0]
	mustEq(t, "method kind", method.Kind, gotestspec.KindMethod)
	mustEq(t, "method display", method.Display, "Create")
	mustLen(t, "when blocks", method.Children, 1)

	when := method.Children[0]
	mustEq(t, "when kind", when.Kind, gotestspec.KindBlock)
	mustEq(t, "when display", when.Display, "when email is valid")
	mustLen(t, "it blocks", when.Children, 1)

	it := when.Children[0]
	mustEq(t, "it kind", it.Kind, gotestspec.KindBlock)
	mustEq(t, "it display", it.Display, "creates the user")
	mustEq(t, "it status", it.Status, gotestspec.StatusPass)
}

func (s *TreeTestSuite) TestBuildTree_FixtureHierarchy(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture"}
{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture"}
{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite"}
{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite/TestDispatch"}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite/TestDispatch","Elapsed":0.045}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite","Elapsed":0.05}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture","Elapsed":0.06}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture","Elapsed":0.07}
{"Action":"pass","Package":"example.com/e2e","Elapsed":0.1}`)
	mustLen(t, "packages", tree, 1)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	fixture := tree[0].Nodes[0]
	mustEq(t, "root kind", fixture.Kind, gotestspec.KindFixture)
	mustEq(t, "fixture display", fixture.Display, "Infra")
	mustLen(t, "fixture children", fixture.Children, 1)

	child := fixture.Children[0]
	mustEq(t, "child kind", child.Kind, gotestspec.KindFixture)
	mustEq(t, "child display", child.Display, "API")
	mustLen(t, "child children", child.Children, 1)

	suite := child.Children[0]
	mustEq(t, "suite kind", suite.Kind, gotestspec.KindSuite)
	mustEq(t, "suite display", suite.Display, "Batch")
	mustLen(t, "suite children", suite.Children, 1)

	method := suite.Children[0]
	mustEq(t, "method kind", method.Kind, gotestspec.KindMethod)
	mustEq(t, "method display", method.Display, "Dispatch")
}

func (s *TreeTestSuite) TestBuildTree_FocusedSuite(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestF_PaymentServiceTestSuite"}
{"Action":"run","Package":"p","Test":"TestF_PaymentServiceTestSuite/TestCharge"}
{"Action":"pass","Package":"p","Test":"TestF_PaymentServiceTestSuite/TestCharge","Elapsed":0.045}
{"Action":"pass","Package":"p","Test":"TestF_PaymentServiceTestSuite","Elapsed":0.05}
{"Action":"pass","Package":"p","Elapsed":0.1}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	suite := tree[0].Nodes[0]
	mustEq(t, "focused", suite.Focused, true)
	mustEq(t, "display", suite.Display, "PaymentService")
}

func (s *TreeTestSuite) TestBuildTree_ExcludedSuite(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestX_BrokenTestSuite"}
{"Action":"skip","Package":"p","Test":"TestX_BrokenTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.1}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	suite := tree[0].Nodes[0]
	mustEq(t, "excluded", suite.Excluded, true)
	mustEq(t, "display", suite.Display, "Broken")
	mustEq(t, "status", suite.Status, gotestspec.StatusSkip)
}

func (s *TreeTestSuite) TestCollectStats(t *gotest.T, _ *treeCtx) {
	stats := gotestspec.CollectStats(treeOf(t, `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestA"}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestA","Elapsed":0.01}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestB"}
{"Action":"fail","Package":"p","Test":"TestFooTestSuite/TestB","Elapsed":0.02}
{"Action":"run","Package":"p","Test":"TestBarTestSuite"}
{"Action":"run","Package":"p","Test":"TestBarTestSuite/TestC"}
{"Action":"skip","Package":"p","Test":"TestBarTestSuite/TestC","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite","Elapsed":0.03}
{"Action":"pass","Package":"p","Test":"TestBarTestSuite","Elapsed":0.01}
{"Action":"pass","Package":"p","Elapsed":0.05}`))
	mustEq(t, "suites", stats.Suites, 2)
	mustEq(t, "behaviors", stats.Behaviors, 3)
	mustEq(t, "tests", stats.Tests, 0)
	mustEq(t, "passed", stats.Passed, 1)
	mustEq(t, "failed", stats.Failed, 1)
	mustEq(t, "skipped", stats.Skipped, 1)
}

func (s *TreeTestSuite) TestBuildTree_StdlibTest(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"example.com/pkg","Test":"TestCreateUser"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCreateUser/valid_email"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCreateUser/valid_email","Elapsed":0.003}
{"Action":"run","Package":"example.com/pkg","Test":"TestCreateUser/duplicate_email"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCreateUser/duplicate_email","Elapsed":0.002}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCreateUser","Elapsed":0.006}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.01}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	test := tree[0].Nodes[0]
	mustEq(t, "root kind", test.Kind, gotestspec.KindTest)
	mustEq(t, "display", test.Display, "CreateUser")
	mustLen(t, "subtests", test.Children, 2)
	mustEq(t, "subtest kind", test.Children[0].Kind, gotestspec.KindBlock)
	mustEq(t, "subtest display", test.Children[0].Display, "valid email")
}

func (s *TreeTestSuite) TestCollectStats_Mixed(t *gotest.T, _ *treeCtx) {
	stats := gotestspec.CollectStats(treeOf(t, `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestA"}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestA","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite","Elapsed":0.02}
{"Action":"run","Package":"p","Test":"TestHelper"}
{"Action":"run","Package":"p","Test":"TestHelper/returns_ok"}
{"Action":"pass","Package":"p","Test":"TestHelper/returns_ok","Elapsed":0.001}
{"Action":"run","Package":"p","Test":"TestHelper/handles_error"}
{"Action":"pass","Package":"p","Test":"TestHelper/handles_error","Elapsed":0.001}
{"Action":"pass","Package":"p","Test":"TestHelper","Elapsed":0.003}
{"Action":"pass","Package":"p","Elapsed":0.05}`))
	mustEq(t, "suites", stats.Suites, 1)
	mustEq(t, "behaviors", stats.Behaviors, 1)
	mustEq(t, "tests", stats.Tests, 2)
	mustEq(t, "passed", stats.Passed, 3)
}

func (s *TreeTestSuite) TestCollectStats_StdlibOnly(t *gotest.T, _ *treeCtx) {
	stats := gotestspec.CollectStats(treeOf(t, `{"Action":"run","Package":"p","Test":"TestFoo"}
{"Action":"pass","Package":"p","Test":"TestFoo","Elapsed":0.01}
{"Action":"run","Package":"p","Test":"TestBar"}
{"Action":"pass","Package":"p","Test":"TestBar","Elapsed":0.02}
{"Action":"pass","Package":"p","Elapsed":0.05}`))
	mustEq(t, "suites", stats.Suites, 0)
	mustEq(t, "behaviors", stats.Behaviors, 0)
	mustEq(t, "tests", stats.Tests, 2)
	mustEq(t, "passed", stats.Passed, 2)
}

func (s *TreeTestSuite) TestSplitTestPath(t *gotest.T, _ *treeCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		path string
		want []string
	}{
		{Desc: "single segment", path: "TestFoo", want: []string{"TestFoo"}},
		{Desc: "two segments", path: "TestFoo/bar", want: []string{"TestFoo", "bar"}},
		{Desc: "three segments", path: "TestFoo/bar/baz", want: []string{"TestFoo", "bar", "baz"}},
		{Desc: "consecutive double slash preserved", path: "TestFoo/https://example.com", want: []string{"TestFoo", "https://example.com"}},
		{Desc: "consecutive triple slash preserved", path: "TestFoo/a///b", want: []string{"TestFoo", "a///b"}},
		{Desc: "empty string", path: "", want: []string{}},
		{Desc: "trailing slash", path: "TestFoo/bar/", want: []string{"TestFoo", "bar"}},
		{Desc: "mixed normal and double slash", path: "TestSuite/method/https://host/path", want: []string{"TestSuite", "method", "https://host", "path"}},
	}) {
		got := gotestspec.ExportSplitTestPath(tc.path)
		if len(got) != len(tc.want) {
			fatalf(sub, "splitTestPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				sub.Errorf("splitTestPath(%q)[%d] = %q, want %q", tc.path, i, got[i], tc.want[i])
			}
		}
	}
}

// Go runs a same-named suite from ptest and pxtest twice and appends #01 to
// the subtests of the second run; the tree keeps them as two suites.
func (s *TreeTestSuite) TestBuildTree_DuplicateSuite_PtestPxtest(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"example.com/stdlib","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestCreate"}
{"Action":"pass","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestCreate","Elapsed":0.01}
{"Action":"run","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestReady"}
{"Action":"pass","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestReady","Elapsed":0.01}
{"Action":"pass","Package":"example.com/stdlib","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"run","Package":"example.com/stdlib","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestCreate#01"}
{"Action":"pass","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestCreate#01","Elapsed":0.01}
{"Action":"run","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestReady#01"}
{"Action":"pass","Package":"example.com/stdlib","Test":"TestUnitTestSuite/TestReady#01","Elapsed":0.01}
{"Action":"pass","Package":"example.com/stdlib","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"pass","Package":"example.com/stdlib","Elapsed":0.05}`)
	mustLen(t, "packages", tree, 1)
	pkg := tree[0]
	mustLen(t, "root nodes (two suites, not one merged)", pkg.Nodes, 2)
	suite1, suite2 := pkg.Nodes[0], pkg.Nodes[1]

	mustEq(t, "suite1 kind", suite1.Kind, gotestspec.KindSuite)
	mustEq(t, "suite2 kind", suite2.Kind, gotestspec.KindSuite)
	mustEq(t, "suite1 display", suite1.Display, "Unit")
	mustEq(t, "suite2 display", suite2.Display, "Unit")
	mustLen(t, "suite1 children", suite1.Children, 2)
	mustLen(t, "suite2 children", suite2.Children, 2)

	for _, c := range suite2.Children {
		mustNotContain(t, c.Name, "#", "suite2 child name keeps no dedup suffix")
		mustNotContain(t, c.Display, "#", "suite2 child display keeps no dedup suffix")
	}

	mustEq(t, "suite2 variant", suite2.Variant, 2)
	mustEq(t, "suite2 external", suite2.External, true)
	mustEq(t, "suite1 external", suite1.External, false)
	mustEq(t, "suite1 status", suite1.Status, gotestspec.StatusPass)
	mustEq(t, "suite2 status", suite2.Status, gotestspec.StatusPass)
}

func (s *TreeTestSuite) TestClassify_ParallelMethod(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestMyTestSuite"}
{"Action":"run","Package":"p","Test":"TestMyTestSuite/TestParallelCreate"}
{"Action":"pass","Package":"p","Test":"TestMyTestSuite/TestParallelCreate","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestMyTestSuite","Elapsed":0.02}
{"Action":"pass","Package":"p","Elapsed":0.05}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	mustLen(t, "methods", tree[0].Nodes[0].Children, 1)
	method := tree[0].Nodes[0].Children[0]
	mustEq(t, "kind", method.Kind, gotestspec.KindMethod)
	mustEq(t, "display", method.Display, "ParallelCreate")
}

func (s *TreeTestSuite) TestBuildTree_PackageLevelOutput(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestFoo"}
{"Action":"output","Package":"p","Test":"TestFoo","Output":"=== RUN   TestFoo\n"}
{"Action":"output","Package":"p","Test":"TestFoo","Output":"--- PASS: TestFoo (0.00s)\n"}
{"Action":"pass","Package":"p","Test":"TestFoo","Elapsed":0}
{"Action":"output","Package":"p","Output":"==================\n"}
{"Action":"output","Package":"p","Output":"WARNING: DATA RACE\n"}
{"Action":"output","Package":"p","Output":"Write at 0x00c by goroutine 9:\n"}
{"Action":"output","Package":"p","Output":"==================\n"}
{"Action":"output","Package":"p","Output":"Found 1 data race(s)\n"}
{"Action":"output","Package":"p","Output":"FAIL\tp\t1.0s\n"}
{"Action":"fail","Package":"p","Elapsed":1.0}`)
	mustLen(t, "packages", tree, 1)
	pkg := tree[0]
	mustLen(t, "nodes", pkg.Nodes, 1)
	mustEq(t, "test status", pkg.Nodes[0].Status, gotestspec.StatusPass)
	mustEq(t, "package status", pkg.Status, gotestspec.StatusFail)
	if len(pkg.Output) == 0 {
		fatalf(t, "expected package-level output, got none")
	}
	combined := strings.Join(pkg.Output, "")
	mustContain(t, combined, "WARNING: DATA RACE", "package output carries the race warning")
	mustContain(t, combined, "Found 1 data race(s)", "package output carries the race count")
	mustNotContain(t, combined, "FAIL\tp\t", "summary lines are not package output")
}

func mustInDelta(t *gotest.T, what string, want, got, delta float64) {
	if diff := got - want; diff > delta || diff < -delta {
		t.Errorf("%s = %v, want %v (within %v)", what, got, want, delta)
	}
}

func (s *TreeTestSuite) TestBuildTree_BenchmarkEvents(t *gotest.T, _ *treeCtx) {
	events := []gotestspec.TestEvent{
		{Action: gotestspec.ActionOutput, Package: "p", Test: "BenchmarkFooTestSuite/BenchmarkParse",
			Output: "BenchmarkFooTestSuite/BenchmarkParse-8   \t 1201 \t 985.2 ns/op \t 24 B/op \t 3 allocs/op\n"},
		{Action: gotestspec.ActionBench, Package: "p", Test: "BenchmarkFooTestSuite/BenchmarkParse"},
	}
	pkgs := gotestspec.BuildTree(events)
	mustLen(t, "packages", pkgs, 1)
	mustLen(t, "root nodes", pkgs[0].Nodes, 1)
	mustLen(t, "children", pkgs[0].Nodes[0].Children, 1)
	leaf := pkgs[0].Nodes[0].Children[0]
	mustEq(t, "kind", leaf.Kind, gotestspec.KindBenchmark)
	mustEq(t, "status", leaf.Status, gotestspec.StatusPass)
	mustInDelta(t, "ns/op", 985.2, leaf.NsPerOp, 0.001)
	mustEq(t, "B/op", leaf.BytesPerOp, int64(24))
	mustEq(t, "allocs/op", leaf.AllocsPerOp, int64(3))
	stats := gotestspec.CollectStats(pkgs)
	mustEq(t, "benchmarks", stats.Benchmarks, 1)
	mustEq(t, "behaviors", stats.Behaviors, 0)
}

// test2json "output" events are not guaranteed line-aligned; under real
// subprocess pipe timing a bench result line can arrive split mid-token
// across two consecutive events for the same tagged Test. Metrics must still
// be recovered from the node's joined output, not just the single event that
// happens to complete the line.
func (s *TreeTestSuite) TestBuildTree_BenchmarkOutputSplitAcrossEvents(t *gotest.T, _ *treeCtx) {
	events := []gotestspec.TestEvent{
		{Action: gotestspec.ActionOutput, Package: "p", Test: "BenchmarkFoo", Output: "BenchmarkFoo-8   \t 12"},
		{Action: gotestspec.ActionOutput, Package: "p", Test: "BenchmarkFoo", Output: "01 \t 985.2 ns/op\n"},
		{Action: gotestspec.ActionBench, Package: "p", Test: "BenchmarkFoo"},
	}
	pkgs := gotestspec.BuildTree(events)
	mustLen(t, "packages", pkgs, 1)
	mustLen(t, "root nodes", pkgs[0].Nodes, 1)
	leaf := pkgs[0].Nodes[0]
	mustEq(t, "kind", leaf.Kind, gotestspec.KindBenchmark)
	mustEq(t, "status", leaf.Status, gotestspec.StatusPass)
	mustEq(t, "iterations", leaf.Iterations, 1201)
	mustInDelta(t, "ns/op", 985.2, leaf.NsPerOp, 0.001)
}

// "TestBenchmarkFoo" is a legitimate stdlib test whose name merely starts
// with "Benchmark" after the "Test" prefix is trimmed. It must resolve
// through the ordinary test classification, never the bench branch.
func (s *TreeTestSuite) TestClassify_TopLevelTestNamedBenchmarkIsNotABenchmark(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestBenchmarkFoo"}
{"Action":"pass","Package":"p","Test":"TestBenchmarkFoo","Elapsed":0.01}
{"Action":"pass","Package":"p","Elapsed":0.01}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	node := tree[0].Nodes[0]
	mustEq(t, "kind", node.Kind, gotestspec.KindTest)
	mustEq(t, "display", node.Display, "BenchmarkFoo")
}

// "Benchmarking_the_new_endpoint" starts with "Benchmark" but continues with
// a lowercase letter, so it is an ordinary block name, not a Go benchmark
// identifier.
func (s *TreeTestSuite) TestClassify_NestedBenchmarkingPrefixIsNotABenchmark(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestBar"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestBar/Benchmarking_the_new_endpoint"}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestBar/Benchmarking_the_new_endpoint","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestBar","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite","Elapsed":0.01}
{"Action":"pass","Package":"p","Elapsed":0.01}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	mustLen(t, "methods", tree[0].Nodes[0].Children, 1)
	mustLen(t, "blocks", tree[0].Nodes[0].Children[0].Children, 1)
	block := tree[0].Nodes[0].Children[0].Children[0]
	mustEq(t, "kind", block.Kind, gotestspec.KindBlock)
	mustEq(t, "display", block.Display, "Benchmarking the new endpoint")
}

func (s *TreeTestSuite) TestClassify_NestedBenchmarkName(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/BenchmarkParse"}
{"Action":"output","Package":"p","Test":"TestFooTestSuite/BenchmarkParse","Output":"BenchmarkParse-8   \t 100 \t 10.0 ns/op\n"}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	mustLen(t, "children", tree[0].Nodes[0].Children, 1)
	leaf := tree[0].Nodes[0].Children[0]
	mustEq(t, "kind", leaf.Kind, gotestspec.KindBenchmark)
	mustEq(t, "display", leaf.Display, "Parse")
}
