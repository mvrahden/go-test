// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestspec_test //nolint:fail-guard // ring 0: raw checks only, never a gotest assertion

import (
	"fmt"
	"regexp"
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

func (s *TreeTestSuite) TestBuildTree_NestedNamesLikeSuitesStayBlocks(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestNamesTestSuite"}
{"Action":"run","Package":"p","Test":"TestNamesTestSuite/TestLabels"}
{"Action":"run","Package":"p","Test":"TestNamesTestSuite/TestLabels/ends_in_TestSuite"}
{"Action":"pass","Package":"p","Test":"TestNamesTestSuite/TestLabels/ends_in_TestSuite","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestNamesTestSuite/TestLabels/binds_a_DBFixture"}
{"Action":"pass","Package":"p","Test":"TestNamesTestSuite/TestLabels/binds_a_DBFixture","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestNamesTestSuite/TestLabels","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestNamesTestSuite/UserTestSuite"}
{"Action":"pass","Package":"p","Test":"TestNamesTestSuite/UserTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestNamesTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.1}`)
	suite := tree[0].Nodes[0]
	mustLen(t, "suite children", suite.Children, 2)
	method := suite.Children[0]
	mustLen(t, "method children", method.Children, 2)
	for i, want := range []string{"ends in TestSuite", "binds a DBFixture"} {
		mustEq(t, "label kind", method.Children[i].Kind, gotestspec.KindBlock)
		mustEq(t, "label display", method.Children[i].Display, want)
	}
	// directly under a suite too: only a fixture's test runs suites
	mustEq(t, "row kind", suite.Children[1].Kind, gotestspec.KindBlock)
	mustEq(t, "row display", suite.Children[1].Display, "UserTestSuite")
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
	mustEq(t, "suite2 run (a twin, not a repeat)", suite2.Run, 0)

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

// Suites finish in dispatch order, which parallel processes shuffle; the
// tree lists them by name so two runs of one package render alike.
func (s *TreeTestSuite) TestBuildTree_SuitesSortedByName(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestZetaTestSuite"}
{"Action":"run","Package":"p","Test":"TestZetaTestSuite/TestOne"}
{"Action":"pass","Package":"p","Test":"TestZetaTestSuite/TestOne","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestZetaTestSuite","Elapsed":0.02}
{"Action":"run","Package":"p","Test":"TestAlphaTestSuite"}
{"Action":"run","Package":"p","Test":"TestAlphaTestSuite/TestOne"}
{"Action":"pass","Package":"p","Test":"TestAlphaTestSuite/TestOne","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestAlphaTestSuite","Elapsed":0.02}
{"Action":"run","Package":"p","Test":"TestMidTestSuite"}
{"Action":"pass","Package":"p","Test":"TestMidTestSuite","Elapsed":0.02}
{"Action":"pass","Package":"p","Elapsed":0.05}`)
	mustLen(t, "root nodes", tree[0].Nodes, 3)
	var names []string
	for _, n := range tree[0].Nodes {
		names = append(names, n.Name)
	}
	mustEq(t, "order", strings.Join(names, ","), "TestAlphaTestSuite,TestMidTestSuite,TestZetaTestSuite")
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

// A benchmark method that opens sub-benchmarks reports no result line of its
// own; each sub-benchmark is a benchmark leaf with its own result, and a
// metric the method reports stays on its leaf.
func (s *TreeTestSuite) TestBuildTree_SubBenchmarkLeaves(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"BenchmarkFooTestSuite"}
{"Action":"run","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkSplit"}
{"Action":"run","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkSplit/short"}
{"Action":"output","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkSplit/short","Output":"BenchmarkFooTestSuite/BenchmarkSplit/short-8   \t 100 \t 10.0 ns/op \t 2.500 widgets/op \t 8 B/op \t 1 allocs/op\n"}
{"Action":"run","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkSplit/long"}
{"Action":"output","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkSplit/long","Output":"BenchmarkFooTestSuite/BenchmarkSplit/long-8   \t 50 \t 20.0 ns/op\n"}`)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	mustLen(t, "methods", tree[0].Nodes[0].Children, 1)
	method := tree[0].Nodes[0].Children[0]
	mustEq(t, "method kind", method.Kind, gotestspec.KindBenchmark)
	mustLen(t, "sub-benchmarks", method.Children, 2)
	short := method.Children[0]
	mustEq(t, "leaf kind", short.Kind, gotestspec.KindBenchmark)
	mustEq(t, "leaf display", short.Display, "short")
	mustEq(t, "leaf status", short.Status, gotestspec.StatusPass)
	mustEq(t, "iterations", short.Iterations, 100)
	mustEq(t, "B/op", short.BytesPerOp, int64(8))
	mustEq(t, "allocs/op", short.AllocsPerOp, int64(1))
	mustInDelta(t, "widgets/op", 2.5, short.Metrics["widgets/op"], 0.001)
	mustEq(t, "procs", short.Procs, 8)
	stats := gotestspec.CollectStats(tree)
	mustEq(t, "benchmarks", stats.Benchmarks, 2)
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

// buildFailureStream is what go test -json (Go 1.24+) writes for a package
// whose test build fails, a package whose dependency fails, and a green one.
const buildFailureStream = `{"ImportPath":"m/dep","Action":"build-output","Output":"# m/dep\n"}
{"ImportPath":"m/dep","Action":"build-output","Output":"dep/dep.go:3:27: undefined: missing\n"}
{"ImportPath":"m/dep","Action":"build-fail"}
{"ImportPath":"m/broken [m/broken.test]","Action":"build-output","Output":"# m/broken [m/broken.test]\n"}
{"ImportPath":"m/broken [m/broken.test]","Action":"build-output","Output":"broken/broken_test.go:5:33: undefined: undefinedCall\n"}
{"ImportPath":"m/broken [m/broken.test]","Action":"build-fail"}
{"Action":"start","Package":"m/broken"}
{"Action":"output","Package":"m/broken","Output":"FAIL\tm/broken [build failed]\n"}
{"Action":"fail","Package":"m/broken","Elapsed":0,"FailedBuild":"m/broken [m/broken.test]"}
{"Action":"start","Package":"m/usesdep"}
{"Action":"output","Package":"m/usesdep","Output":"FAIL\tm/usesdep [build failed]\n"}
{"Action":"fail","Package":"m/usesdep","Elapsed":0,"FailedBuild":"m/dep"}
{"Action":"run","Package":"m/ok","Test":"TestOK"}
{"Action":"pass","Package":"m/ok","Test":"TestOK","Elapsed":0}
{"Action":"pass","Package":"m/ok","Elapsed":0.006}`

func (s *TreeTestSuite) TestBuildTree_BuildFailure(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, buildFailureStream)
	mustLen(t, "packages", tree, 3)
	broken, ok, usesdep := tree[0], tree[1], tree[2]
	mustEq(t, "first package", broken.Path, "m/broken")
	mustEq(t, "second package", ok.Path, "m/ok")
	mustEq(t, "third package", usesdep.Path, "m/usesdep")

	mustEq(t, "broken status", broken.Status, gotestspec.StatusFail)
	mustContain(t, strings.Join(broken.Output, ""), "broken/broken_test.go:5:33: undefined: undefinedCall", "a failed build carries its diagnostics")
	mustNotContain(t, strings.Join(broken.Output, ""), "dep/dep.go", "another build's diagnostics stay with that build")

	mustEq(t, "usesdep status", usesdep.Status, gotestspec.StatusFail)
	mustContain(t, strings.Join(usesdep.Output, ""), "dep/dep.go:3:27: undefined: missing", "a failed dependency's diagnostics reach the package it failed")

	mustEq(t, "ok status", ok.Status, gotestspec.StatusPass)
	mustLen(t, "ok output", ok.Output, 0)
}

func (s *TreeTestSuite) TestBuildTree_FailureFailsEveryAncestorButSkipStays(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"example.com/pkg","Test":"TestATestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestATestSuite/TestOne"}
{"Action":"run","Package":"example.com/pkg","Test":"TestATestSuite/TestOne/when_x"}
{"Action":"run","Package":"example.com/pkg","Test":"TestATestSuite/TestOne/when_x/fails"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestATestSuite/TestOne/when_x/fails","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestATestSuite/TestOne/when_x","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestATestSuite/TestOne","Elapsed":0.001}
{"Action":"run","Package":"example.com/pkg","Test":"TestATestSuite/TestTwo"}
{"Action":"run","Package":"example.com/pkg","Test":"TestATestSuite/TestTwo/skips"}
{"Action":"skip","Package":"example.com/pkg","Test":"TestATestSuite/TestTwo/skips","Elapsed":0}
{"Action":"pass","Package":"example.com/pkg","Test":"TestATestSuite/TestTwo","Elapsed":0}
{"Action":"pass","Package":"example.com/pkg","Test":"TestATestSuite","Elapsed":0.002}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.003}`)
	mustLen(t, "packages", tree, 1)
	mustEq(t, "package status", tree[0].Status, gotestspec.StatusFail)
	suite := tree[0].Nodes[0]
	mustEq(t, "suite status", suite.Status, gotestspec.StatusFail)
	mustEq(t, "method status", suite.Children[0].Status, gotestspec.StatusFail)
	mustEq(t, "when status", suite.Children[0].Children[0].Status, gotestspec.StatusFail)
	mustEq(t, "skipped method status", suite.Children[1].Status, gotestspec.StatusPass)
	stats := gotestspec.CollectStats(tree)
	mustEq(t, "failed", stats.Failed, 1)
	mustEq(t, "skipped", stats.Skipped, 1)
}

func (s *TreeTestSuite) TestBuildTree_DuplicateSuiteMethodsWithoutSuffix(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"example.com/auth","Test":"TestTokenTestSuite"}
{"Action":"run","Package":"example.com/auth","Test":"TestTokenTestSuite/TestValidate"}
{"Action":"pass","Package":"example.com/auth","Test":"TestTokenTestSuite/TestValidate","Elapsed":0}
{"Action":"pass","Package":"example.com/auth","Test":"TestTokenTestSuite","Elapsed":0}
{"Action":"run","Package":"example.com/auth","Test":"TestTokenTestSuite"}
{"Action":"run","Package":"example.com/auth","Test":"TestTokenTestSuite/TestSecret"}
{"Action":"fail","Package":"example.com/auth","Test":"TestTokenTestSuite/TestSecret","Elapsed":0}
{"Action":"fail","Package":"example.com/auth","Test":"TestTokenTestSuite","Elapsed":0}
{"Action":"fail","Package":"example.com/auth","Elapsed":0}`)
	mustLen(t, "packages", tree, 1)
	mustLen(t, "root nodes", tree[0].Nodes, 2)
	internal, external := tree[0].Nodes[0], tree[0].Nodes[1]
	mustLen(t, "internal methods", internal.Children, 1)
	mustEq(t, "internal method", internal.Children[0].Name, "TestValidate")
	mustEq(t, "internal status", internal.Status, gotestspec.StatusPass)
	mustLen(t, "external methods", external.Children, 1)
	mustEq(t, "external method", external.Children[0].Name, "TestSecret")
	mustEq(t, "external status", external.Status, gotestspec.StatusFail)
	mustEq(t, "external", external.External, true)
}

// methodOf returns the only suite's only method, halting otherwise.
func methodOf(t *gotest.T, tree []*gotestspec.Package) *gotestspec.Node {
	mustLen(t, "packages", tree, 1)
	mustLen(t, "root nodes", tree[0].Nodes, 1)
	mustLen(t, "methods", tree[0].Nodes[0].Children, 1)
	return tree[0].Nodes[0].Children[0]
}

func (s *TreeTestSuite) TestATrailingSlashLabelIsOneBehavior(t *gotest.T, _ *treeCtx) {
	m := methodOf(t, treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/gets_./"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/gets_./","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.01}`))
	mustLen(t, "behaviors", m.Children, 1)
	mustEq(t, "behavior name", m.Children[0].Name, "gets_./")
	mustEq(t, "behavior status", m.Children[0].Status, gotestspec.StatusPass)
}

func (s *TreeTestSuite) TestALeadingSlashLabelIsOneBehavior(t *gotest.T, _ *treeCtx) {
	m := methodOf(t, treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM//abs/pkg"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM//abs/pkg","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.01}`))
	mustEq(t, "method name", m.Name, "TestM")
	mustLen(t, "behaviors", m.Children, 1)
	mustEq(t, "behavior name", m.Children[0].Name, "/abs/pkg")
	mustEq(t, "behavior status", m.Children[0].Status, gotestspec.StatusPass)
}

func (s *TreeTestSuite) TestAGroupEndingInASlashKeepsItsChild(t *gotest.T, _ *treeCtx) {
	m := methodOf(t, treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/reads_specs/"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/reads_specs//holds"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/reads_specs//holds","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/reads_specs/","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.01}`))
	mustLen(t, "groups", m.Children, 1)
	group := m.Children[0]
	mustEq(t, "group name", group.Name, "reads_specs/")
	mustLen(t, "group children", group.Children, 1)
	mustEq(t, "child name", group.Children[0].Name, "holds")
	mustEq(t, "child status", group.Children[0].Status, gotestspec.StatusPass)
}

func (s *TreeTestSuite) TestADotThenDotPkgAreSiblings(t *gotest.T, _ *treeCtx) {
	m := methodOf(t, treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/."}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/.","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/./pkg"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/./pkg","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.01}`))
	mustLen(t, "siblings", m.Children, 2)
	mustEq(t, "first name", m.Children[0].Name, ".")
	mustEq(t, "second name", m.Children[1].Name, "./pkg")
	mustEq(t, "first status", m.Children[0].Status, gotestspec.StatusPass)
	mustEq(t, "second status", m.Children[1].Status, gotestspec.StatusPass)
}

// The external twin's TestCreate is written TestCreate#01, its own TestRemove
// without a suffix; each lands in its own suite.
func (s *TreeTestSuite) TestTwinSuitesKeepTheirOwnMethods(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate"}
{"Action":"pass","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate","Elapsed":0.01}
{"Action":"pass","Package":"example.com/unit","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate#01"}
{"Action":"pass","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate#01","Elapsed":0.01}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite/TestRemove"}
{"Action":"fail","Package":"example.com/unit","Test":"TestUnitTestSuite/TestRemove","Elapsed":0.01}
{"Action":"fail","Package":"example.com/unit","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"fail","Package":"example.com/unit","Elapsed":0.05}`)
	mustLen(t, "packages", tree, 1)
	mustLen(t, "root nodes", tree[0].Nodes, 2)
	internal, external := tree[0].Nodes[0], tree[0].Nodes[1]
	mustEq(t, "internal external", internal.External, false)
	mustEq(t, "internal variant", internal.Variant, 0)
	mustLen(t, "internal methods", internal.Children, 1)
	mustEq(t, "internal method", internal.Children[0].Name, "TestCreate")
	mustEq(t, "internal status", internal.Status, gotestspec.StatusPass)
	mustEq(t, "external external", external.External, true)
	mustEq(t, "external variant", external.Variant, 2)
	mustLen(t, "external methods", external.Children, 2)
	mustEq(t, "external first method", external.Children[0].Name, "TestCreate")
	mustEq(t, "external second method", external.Children[1].Name, "TestRemove")
	mustEq(t, "external first method reported", external.Children[0].Reported, "TestCreate#01")
	mustEq(t, "external second method reported", external.Children[1].Reported, "")
	mustEq(t, "internal method reported", internal.Children[0].Reported, "")
	mustEq(t, "external status", external.Status, gotestspec.StatusFail)
}

func (s *TreeTestSuite) TestAnOrphanedSubtestStillLands(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/a/b"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/a/b","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.01}`)
	m := methodOf(t, tree)
	mustEq(t, "suite status", tree[0].Nodes[0].Status, gotestspec.StatusPass)
	mustEq(t, "method name", m.Name, "TestM")
	mustLen(t, "method children", m.Children, 1)
	a := m.Children[0]
	mustEq(t, "group name", a.Name, "a")
	mustLen(t, "group children", a.Children, 1)
	mustEq(t, "leaf name", a.Children[0].Name, "b")
	mustEq(t, "leaf status", a.Children[0].Status, gotestspec.StatusPass)
}

func (s *TreeTestSuite) TestFuzzSeedsKeepTheirNames(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"FuzzCodecTestSuite_FuzzRoundTrip"}
{"Action":"run","Package":"p","Test":"FuzzCodecTestSuite_FuzzRoundTrip/seed#0"}
{"Action":"pass","Package":"p","Test":"FuzzCodecTestSuite_FuzzRoundTrip/seed#0","Elapsed":0}
{"Action":"pass","Package":"p","Test":"FuzzCodecTestSuite_FuzzRoundTrip","Elapsed":0.01}
{"Action":"pass","Package":"p","Elapsed":0.02}`)
	target := methodOf(t, tree)
	mustEq(t, "target kind", target.Kind, gotestspec.KindFuzz)
	mustLen(t, "seeds", target.Children, 1)
	mustEq(t, "seed name", target.Children[0].Name, "seed#0")
	mustEq(t, "seed display", target.Children[0].Display, "seed #1")
}

// A child reported before its parents is hung on guessed levels; the
// parents' runs, arriving later, put each where it sits and drop the guess.
func (s *TreeTestSuite) TestAParentsLateRunReplacesItsGuess(t *gotest.T, _ *treeCtx) {
	m := methodOf(t, treeOf(t, `{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/x/y/leaf"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/x/y/leaf","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/x/y"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/x/y","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.01}`))
	mustLen(t, "method children", m.Children, 1)
	group := m.Children[0]
	mustEq(t, "group name", group.Name, "x/y")
	mustEq(t, "group status", group.Status, gotestspec.StatusPass)
	mustLen(t, "group children", group.Children, 1)
	mustEq(t, "leaf name", group.Children[0].Name, "leaf")
}

// stampedTwins is a stamped stream: a suite of both test packages, and a
// paused "." beside "./pkg", which only the stream's order told apart.
var stampedTwins = []string{
	`{"Action":"run","Package":"p","Test":"TestSTestSuite","Gotest":{"Subtest":"TestSTestSuite"}}`,
	`{"Action":"run","Package":"p","Test":"TestSTestSuite/TestCreate","Gotest":{"Subtest":"TestCreate"}}`,
	`{"Action":"run","Package":"p","Test":"TestSTestSuite/TestCreate/.","Gotest":{"Subtest":"."}}`,
	`{"Action":"pause","Package":"p","Test":"TestSTestSuite/TestCreate/."}`,
	`{"Action":"run","Package":"p","Test":"TestSTestSuite/TestCreate/./pkg","Gotest":{"Subtest":"./pkg"}}`,
	`{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestCreate/./pkg","Elapsed":0}`,
	`{"Action":"cont","Package":"p","Test":"TestSTestSuite/TestCreate/."}`,
	`{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestCreate/.","Elapsed":0}`,
	`{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestCreate","Elapsed":0}`,
	`{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}`,
	`{"Action":"run","Package":"p","Test":"TestSTestSuite","Gotest":{"Subtest":"TestSTestSuite","Instance":2}}`,
	`{"Action":"run","Package":"p","Test":"TestSTestSuite/TestCreate#01","Gotest":{"Subtest":"TestCreate#01","Repeat":1,"Instance":2}}`,
	`{"Action":"run","Package":"p","Test":"TestSTestSuite/TestCreate#01/when_x","Gotest":{"Subtest":"when_x","Instance":2}}`,
	`{"Action":"output","Package":"p","Test":"TestSTestSuite/TestCreate#01/when_x","Output":"    boom\n","Gotest":{"Instance":2}}`,
	`{"Action":"fail","Package":"p","Test":"TestSTestSuite/TestCreate#01/when_x","Elapsed":0,"Gotest":{"Instance":2}}`,
	`{"Action":"fail","Package":"p","Test":"TestSTestSuite/TestCreate#01","Elapsed":0,"Gotest":{"Instance":2}}`,
	`{"Action":"fail","Package":"p","Test":"TestSTestSuite","Elapsed":0,"Gotest":{"Instance":2}}`,
	`{"Action":"fail","Package":"p","Elapsed":0.01}`,
}

// hoistRuns puts every run event first, as a stream composed of several
// runs can.
func hoistRuns(lines []string) []string {
	var runs, rest []string
	for _, l := range lines {
		if strings.Contains(l, `"Action":"run"`) {
			runs = append(runs, l)
		} else {
			rest = append(rest, l)
		}
	}
	return append(runs, rest...)
}

func renderedTree(t *gotest.T, lines []string) string {
	var b strings.Builder
	gotestspec.RenderJSON(&b, treeOf(t, strings.Join(lines, "\n")))
	return b.String()
}

func (s *TreeTestSuite) TestAStampedStreamNeedsNoOrder(t *gotest.T, _ *treeCtx) {
	want := renderedTree(t, stampedTwins)
	mustEq(t, "hoisted stamped tree", renderedTree(t, hoistRuns(stampedTwins)), want)

	unstamped := make([]string, len(stampedTwins))
	for i, l := range stampedTwins {
		unstamped[i] = regexp.MustCompile(`,"Gotest":\{[^{}]*\}`).ReplaceAllString(l, "")
	}
	mustEq(t, "unstamped tree in stream order", renderedTree(t, unstamped), want)
	// The hoisting matters: without stamps it misplaces.
	if renderedTree(t, hoistRuns(unstamped)) == want {
		t.Errorf("hoisting an unstamped stream changed nothing; the test does not reorder")
	}
}

// A package that is stamped in part, as a stream composed from an older and
// a newer CLI is, places its unstamped events by the fallback split.
func (s *TreeTestSuite) TestAnUnstampedEventOfAStampedPackageFallsBack(t *gotest.T, _ *treeCtx) {
	tree := treeOf(t, `{"Action":"run","Package":"p","Test":"TestATestSuite","Gotest":{"Subtest":"TestATestSuite"}}
{"Action":"pass","Package":"p","Test":"TestATestSuite","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestSTestSuite"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM"}
{"Action":"run","Package":"p","Test":"TestSTestSuite/TestM/a/b"}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM/a/b","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite/TestM","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestSTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.01}`)
	mustLen(t, "root nodes", tree[0].Nodes, 2)
	suite := tree[0].Nodes[1]
	mustEq(t, "suite name", suite.Name, "TestSTestSuite")
	mustEq(t, "suite status", suite.Status, gotestspec.StatusPass)
	mustLen(t, "methods", suite.Children, 1)
	m := suite.Children[0]
	mustLen(t, "method children", m.Children, 1)
	mustEq(t, "split level", m.Children[0].Name, "a")
	mustLen(t, "split children", m.Children[0].Children, 1)
	mustEq(t, "split leaf", m.Children[0].Children[0].Name, "b")
	mustEq(t, "leaf status", m.Children[0].Children[0].Status, gotestspec.StatusPass)
}

// A sub-benchmark whose own name ends in a number: a result line that gives
// the name exactly carries no GOMAXPROCS suffix.
func (s *TreeTestSuite) TestBuildTree_SubBenchmarkNamedWithANumber(t *gotest.T, _ *treeCtx) {
	stream := func(reported string) string {
		return `{"Action":"run","Package":"p","Test":"BenchmarkFooTestSuite"}
{"Action":"run","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkCopy"}
{"Action":"run","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkCopy/size-1024"}
{"Action":"output","Package":"p","Test":"BenchmarkFooTestSuite/BenchmarkCopy/size-1024","Output":"` + reported + `   \t 100 \t 10.0 ns/op\n"}`
	}
	leafOf := func(tree []*gotestspec.Package) *gotestspec.Node {
		mustLen(t, "root nodes", tree[0].Nodes, 1)
		mustLen(t, "methods", tree[0].Nodes[0].Children, 1)
		mustLen(t, "sub-benchmarks", tree[0].Nodes[0].Children[0].Children, 1)
		return tree[0].Nodes[0].Children[0].Children[0]
	}

	bare := leafOf(treeOf(t, stream("BenchmarkFooTestSuite/BenchmarkCopy/size-1024")))
	mustEq(t, "display", bare.Display, "size-1024")
	mustEq(t, "iterations", bare.Iterations, 100)
	mustEq(t, "procs without a suffix", bare.Procs, 0)

	suffixed := leafOf(treeOf(t, stream("BenchmarkFooTestSuite/BenchmarkCopy/size-1024-8")))
	mustEq(t, "procs with a suffix", suffixed.Procs, 8)
}

// suiteRun is one top-level run of a suite in an unstamped stream: its
// methods, each passing unless fails names it.
func suiteRun(suite string, methods []string, fails map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"Action":"run","Package":"p","Test":%q}`+"\n", suite)
	status := "pass"
	for _, m := range methods {
		fmt.Fprintf(&b, `{"Action":"run","Package":"p","Test":%q}`+"\n", suite+"/"+m)
		action := "pass"
		if msg, ok := fails[m]; ok {
			fmt.Fprintf(&b, `{"Action":"output","Package":"p","Test":%q,"Output":%q}`+"\n", suite+"/"+m, msg)
			action, status = "fail", "fail"
		}
		fmt.Fprintf(&b, `{"Action":%q,"Package":"p","Test":%q,"Elapsed":0}`+"\n", action, suite+"/"+m)
	}
	fmt.Fprintf(&b, `{"Action":%q,"Package":"p","Test":%q,"Elapsed":0}`+"\n", status, suite)
	return b.String()
}

func runsOf(t *gotest.T, tree []*gotestspec.Package, want int) []*gotestspec.Node {
	mustLen(t, "packages", tree, 1)
	mustLen(t, "root nodes", tree[0].Nodes, want)
	return tree[0].Nodes
}

func (s *TreeTestSuite) TestThreeRunsOfOneSuite(t *gotest.T, _ *treeCtx) {
	methods := []string{"TestFailsOnItsSecondRun", "TestAlwaysPasses"}
	tree := treeOf(t, suiteRun("TestFlakyTestSuite", methods, nil)+
		suiteRun("TestFlakyTestSuite", methods, map[string]string{"TestFailsOnItsSecondRun": "    flaky_test.go:12: second run\n"})+
		suiteRun("TestFlakyTestSuite", methods, nil))
	nodes := runsOf(t, tree, 3)
	for i, n := range nodes {
		mustEq(t, fmt.Sprintf("run %d external", i+1), n.External, false)
		mustEq(t, fmt.Sprintf("run %d run", i+1), n.Run, i+1)
		mustEq(t, fmt.Sprintf("run %d runs", i+1), n.Runs, 3)
		mustLen(t, fmt.Sprintf("run %d methods", i+1), n.Children, 2)
	}
	second := nodes[1].Children[0]
	mustEq(t, "the second run's failing method", second.Status, gotestspec.StatusFail)
	mustContain(t, strings.Join(second.Output, ""), "second run", "the failing method keeps its output")
	mustEq(t, "the first run's method", nodes[0].Children[0].Status, gotestspec.StatusPass)
	mustEq(t, "the third run's method", nodes[2].Children[0].Status, gotestspec.StatusPass)
	stats := gotestspec.CollectStats(tree)
	mustEq(t, "failed", stats.Failed, 1)
	mustEq(t, "passed", stats.Passed, 5)
}

func (s *TreeTestSuite) TestTwinsRunTwice(t *gotest.T, _ *treeCtx) {
	pair := suiteRun("TestUnitTestSuite", []string{"TestCreate"}, nil) + suiteRun("TestUnitTestSuite", []string{"TestCreate#01"}, nil)
	nodes := runsOf(t, treeOf(t, pair+pair), 4)
	for i, want := range []struct {
		external  bool
		run, runs int
	}{{false, 1, 2}, {true, 1, 2}, {false, 2, 2}, {true, 2, 2}} {
		mustEq(t, fmt.Sprintf("node %d external", i+1), nodes[i].External, want.external)
		mustEq(t, fmt.Sprintf("node %d run", i+1), nodes[i].Run, want.run)
		mustEq(t, fmt.Sprintf("node %d runs", i+1), nodes[i].Runs, want.runs)
	}
}

func (s *TreeTestSuite) TestTwinsTheSourceNames(t *gotest.T, _ *treeCtx) {
	guarded := `{"Action":"run","Package":"p","Test":"TestGuardedTestSuite"}
{"Action":"skip","Package":"p","Test":"TestGuardedTestSuite","Elapsed":0}
`
	events, err := gotestspec.ParseEvents(strings.NewReader(guarded + guarded))
	if err != nil {
		fatalf(t, "ParseEvents: %v", err)
	}
	named := gotestspec.BuildTree(events, gotestspec.WithDeclarations(gotestspec.DeclarationIndex{"p": {gotestspec.DeclKey("TestGuardedTestSuite"): {Twin: true}}}))
	twins := runsOf(t, named, 2)
	mustEq(t, "internal twin", twins[0].External, false)
	mustEq(t, "external twin", twins[1].External, true)
	mustEq(t, "twin run", twins[0].Run, 0)
	mustEq(t, "twin run", twins[1].Run, 0)

	unnamed := runsOf(t, gotestspec.BuildTree(events), 2)
	mustEq(t, "first run external", unnamed[0].External, false)
	mustEq(t, "second run external", unnamed[1].External, false)
	mustEq(t, "first run", unnamed[0].Run, 1)
	mustEq(t, "second run", unnamed[1].Run, 2)
	mustEq(t, "runs", unnamed[1].Runs, 2)
}

func (s *TreeTestSuite) TestRepeatedSubtestNamesAreNotATwin(t *gotest.T, _ *treeCtx) {
	run := suiteRun("TestX", []string{"a", "a#01"}, nil)
	nodes := runsOf(t, treeOf(t, run+run), 2)
	mustEq(t, "first external", nodes[0].External, false)
	mustEq(t, "second external", nodes[1].External, false)
	mustEq(t, "first run", nodes[0].Run, 1)
	mustEq(t, "second run", nodes[1].Run, 2)
}

// A stdlib test both test packages declare has no children for the stream
// rules to read: only the source says it is a twin pair.
func (s *TreeTestSuite) TestStdlibTwins(t *gotest.T, _ *treeCtx) {
	one := `{"Action":"run","Package":"p","Test":"TestFoo"}
{"Action":"pass","Package":"p","Test":"TestFoo","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestFoo"}
{"Action":"fail","Package":"p","Test":"TestFoo","Elapsed":0}
`
	decls := gotestspec.WithDeclarations(gotestspec.DeclarationIndex{"p": {gotestspec.DeclKey("TestFoo"): {Twin: true}}})
	build := func(stream string) []*gotestspec.Package {
		events, err := gotestspec.ParseEvents(strings.NewReader(stream))
		if err != nil {
			fatalf(t, "ParseEvents: %v", err)
		}
		return gotestspec.BuildTree(events, decls)
	}

	once := runsOf(t, build(one), 2)
	mustEq(t, "internal", once[0].External, false)
	mustEq(t, "external", once[1].External, true)
	mustEq(t, "runs of a single run", once[1].Runs, 0)

	twice := runsOf(t, build(one+one), 4)
	for i, want := range []struct {
		external bool
		run      int
	}{{false, 1}, {true, 1}, {false, 2}, {true, 2}} {
		mustEq(t, fmt.Sprintf("node %d external", i+1), twice[i].External, want.external)
		mustEq(t, fmt.Sprintf("node %d run", i+1), twice[i].Run, want.run)
		mustEq(t, fmt.Sprintf("node %d runs", i+1), twice[i].Runs, 2)
	}
}

// A skip's message and a line logged before a skip without one read alike;
// only the source's skip calls, or gotest's harness, tell them apart.
func (s *TreeTestSuite) TestSkipReasons(t *gotest.T, _ *treeCtx) {
	stream := `{"Action":"run","Package":"p","Test":"TestLogThenSkip"}
{"Action":"output","Package":"p","Test":"TestLogThenSkip","Output":"    skip_test.go:6: connecting to the database\n"}
{"Action":"output","Package":"p","Test":"TestLogThenSkip","Output":"    skip_test.go:7: no database here\n"}
{"Action":"output","Package":"p","Test":"TestLogThenSkip","Output":"--- SKIP: TestLogThenSkip (0.00s)\n"}
{"Action":"skip","Package":"p","Test":"TestLogThenSkip","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestLogThenSkipNow"}
{"Action":"output","Package":"p","Test":"TestLogThenSkipNow","Output":"    skip_test.go:11: checked the flag\n"}
{"Action":"output","Package":"p","Test":"TestLogThenSkipNow","Output":"--- SKIP: TestLogThenSkipNow (0.00s)\n"}
{"Action":"skip","Package":"p","Test":"TestLogThenSkipNow","Elapsed":0}
{"Action":"run","Package":"p","Test":"TestGuardedTestSuite"}
{"Action":"output","Package":"p","Test":"TestGuardedTestSuite","Output":"    gotest_psuite_test.go:25: suite guard: DATABASE_URL not set\n"}
{"Action":"skip","Package":"p","Test":"TestGuardedTestSuite","Elapsed":0}
`
	events, err := gotestspec.ParseEvents(strings.NewReader(stream))
	if err != nil {
		fatalf(t, "ParseEvents: %v", err)
	}
	reasons := func(opts ...gotestspec.BuildOption) map[string]string {
		out := map[string]string{}
		for _, n := range gotestspec.BuildTree(events, opts...)[0].Nodes {
			out[n.Name] = n.SkipReason
		}
		return out
	}
	calls := gotestspec.WithSkipCalls(gotestspec.SkipCalls{"p": {"skip_test.go:7": true, "skip_test.go:12": true}})

	withSource := reasons(calls)
	mustEq(t, "the skip's message, not the line logged before it", withSource["TestLogThenSkip"], "no database here")
	mustEq(t, "no reason for a skip that gave none", withSource["TestLogThenSkipNow"], "")
	mustEq(t, "the harness's own skip", withSource["TestGuardedTestSuite"], "suite guard: DATABASE_URL not set")

	without := reasons()
	mustEq(t, "no reason when the source cannot say", without["TestLogThenSkip"], "")
	mustEq(t, "the harness's own skip without the source", without["TestGuardedTestSuite"], "suite guard: DATABASE_URL not set")
}
