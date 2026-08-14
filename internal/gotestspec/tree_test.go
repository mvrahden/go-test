package gotestspec //nolint:stdlib-test

import (
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestBuildTree_SuiteHierarchy(t *testing.T) {
	input := `{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate"}
{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid"}
{"Action":"run","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid/creates_the_user"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid/creates_the_user","Elapsed":0.008}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate/when_email_is_valid","Elapsed":0.009}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite/TestCreate","Elapsed":0.01}
{"Action":"pass","Package":"example.com/pkg","Test":"TestUserServiceTestSuite","Elapsed":0.011}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.5}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)

	tree := BuildTree(events)
	gotest.Len(t, tree, 1, "packages")
	pkg := tree[0]
	gotest.Equal(t, "example.com/pkg", pkg.Path)
	gotest.Len(t, pkg.Nodes, 1, "root nodes")

	suite := pkg.Nodes[0]
	gotest.Equal(t, KindSuite, suite.Kind)
	gotest.Equal(t, "UserService", suite.Display)

	gotest.Len(t, suite.Children, 1, "methods")
	method := suite.Children[0]
	gotest.Equal(t, KindMethod, method.Kind)
	gotest.Equal(t, "Create", method.Display)

	gotest.Len(t, method.Children, 1, "when blocks")
	when := method.Children[0]
	gotest.Equal(t, KindBlock, when.Kind)
	gotest.Equal(t, "when email is valid", when.Display)

	gotest.Len(t, when.Children, 1, "it blocks")
	it := when.Children[0]
	gotest.Equal(t, KindBlock, it.Kind)
	gotest.Equal(t, "creates the user", it.Display)
	gotest.Equal(t, StatusPass, it.Status)
}

func TestBuildTree_FixtureHierarchy(t *testing.T) {
	input := `{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture"}
{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture"}
{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite"}
{"Action":"run","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite/TestDispatch"}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite/TestDispatch","Elapsed":0.045}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture/BatchTestSuite","Elapsed":0.05}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture/APIFixture","Elapsed":0.06}
{"Action":"pass","Package":"example.com/e2e","Test":"Test_InfraFixture","Elapsed":0.07}
{"Action":"pass","Package":"example.com/e2e","Elapsed":0.1}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)

	tree := BuildTree(events)
	pkg := tree[0]
	fixture := pkg.Nodes[0]
	gotest.Equal(t, KindFixture, fixture.Kind)
	gotest.Equal(t, "Infra", fixture.Display)

	child := fixture.Children[0]
	gotest.Equal(t, KindFixture, child.Kind)
	gotest.Equal(t, "API", child.Display)

	suite := child.Children[0]
	gotest.Equal(t, KindSuite, suite.Kind)
	gotest.Equal(t, "Batch", suite.Display)

	method := suite.Children[0]
	gotest.Equal(t, KindMethod, method.Kind)
	gotest.Equal(t, "Dispatch", method.Display)
}

func TestBuildTree_FocusedSuite(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestF_PaymentServiceTestSuite"}
{"Action":"run","Package":"p","Test":"TestF_PaymentServiceTestSuite/TestCharge"}
{"Action":"pass","Package":"p","Test":"TestF_PaymentServiceTestSuite/TestCharge","Elapsed":0.045}
{"Action":"pass","Package":"p","Test":"TestF_PaymentServiceTestSuite","Elapsed":0.05}
{"Action":"pass","Package":"p","Elapsed":0.1}`

	events, _ := ParseEvents(strings.NewReader(input))
	tree := BuildTree(events)

	suite := tree[0].Nodes[0]
	gotest.True(t, suite.Focused, "expected suite to be focused")
	gotest.Equal(t, "PaymentService", suite.Display)
}

func TestBuildTree_ExcludedSuite(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestX_BrokenTestSuite"}
{"Action":"skip","Package":"p","Test":"TestX_BrokenTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.1}`

	events, _ := ParseEvents(strings.NewReader(input))
	tree := BuildTree(events)

	suite := tree[0].Nodes[0]
	gotest.True(t, suite.Excluded, "expected suite to be excluded")
	gotest.Equal(t, "Broken", suite.Display)
	gotest.Equal(t, StatusSkip, suite.Status)
}

func TestCollectStats(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestA"}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestA","Elapsed":0.01}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestB"}
{"Action":"fail","Package":"p","Test":"TestFooTestSuite/TestB","Elapsed":0.02}
{"Action":"run","Package":"p","Test":"TestBarTestSuite"}
{"Action":"run","Package":"p","Test":"TestBarTestSuite/TestC"}
{"Action":"skip","Package":"p","Test":"TestBarTestSuite/TestC","Elapsed":0}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite","Elapsed":0.03}
{"Action":"pass","Package":"p","Test":"TestBarTestSuite","Elapsed":0.01}
{"Action":"pass","Package":"p","Elapsed":0.05}`

	events, _ := ParseEvents(strings.NewReader(input))
	tree := BuildTree(events)
	stats := CollectStats(tree)

	gotest.Equal(t, 2, stats.Suites, "suites")
	gotest.Equal(t, 3, stats.Behaviors, "behaviors")
	gotest.Equal(t, 0, stats.Tests, "tests")
	gotest.Equal(t, 1, stats.Passed, "passed")
	gotest.Equal(t, 1, stats.Failed, "failed")
	gotest.Equal(t, 1, stats.Skipped, "skipped")
}

func TestBuildTree_StdlibTest(t *testing.T) {
	input := `{"Action":"run","Package":"example.com/pkg","Test":"TestCreateUser"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCreateUser/valid_email"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCreateUser/valid_email","Elapsed":0.003}
{"Action":"run","Package":"example.com/pkg","Test":"TestCreateUser/duplicate_email"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCreateUser/duplicate_email","Elapsed":0.002}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCreateUser","Elapsed":0.006}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.01}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)

	tree := BuildTree(events)
	pkg := tree[0]
	gotest.Len(t, pkg.Nodes, 1, "root nodes")

	test := pkg.Nodes[0]
	gotest.Equal(t, KindTest, test.Kind)
	gotest.Equal(t, "CreateUser", test.Display)
	gotest.Len(t, test.Children, 2, "subtests")
	gotest.Equal(t, KindBlock, test.Children[0].Kind)
	gotest.Equal(t, "valid email", test.Children[0].Display)
}

func TestCollectStats_Mixed(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestA"}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestA","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite","Elapsed":0.02}
{"Action":"run","Package":"p","Test":"TestHelper"}
{"Action":"run","Package":"p","Test":"TestHelper/returns_ok"}
{"Action":"pass","Package":"p","Test":"TestHelper/returns_ok","Elapsed":0.001}
{"Action":"run","Package":"p","Test":"TestHelper/handles_error"}
{"Action":"pass","Package":"p","Test":"TestHelper/handles_error","Elapsed":0.001}
{"Action":"pass","Package":"p","Test":"TestHelper","Elapsed":0.003}
{"Action":"pass","Package":"p","Elapsed":0.05}`

	events, _ := ParseEvents(strings.NewReader(input))
	tree := BuildTree(events)
	stats := CollectStats(tree)

	gotest.Equal(t, 1, stats.Suites, "suites")
	gotest.Equal(t, 1, stats.Behaviors, "behaviors")
	gotest.Equal(t, 2, stats.Tests, "tests")
	gotest.Equal(t, 3, stats.Passed, "passed")
}

func TestCollectStats_StdlibOnly(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestFoo"}
{"Action":"pass","Package":"p","Test":"TestFoo","Elapsed":0.01}
{"Action":"run","Package":"p","Test":"TestBar"}
{"Action":"pass","Package":"p","Test":"TestBar","Elapsed":0.02}
{"Action":"pass","Package":"p","Elapsed":0.05}`

	events, _ := ParseEvents(strings.NewReader(input))
	tree := BuildTree(events)
	stats := CollectStats(tree)

	gotest.Equal(t, 0, stats.Suites, "suites")
	gotest.Equal(t, 0, stats.Behaviors, "behaviors")
	gotest.Equal(t, 2, stats.Tests, "tests")
	gotest.Equal(t, 2, stats.Passed, "passed")
}

func TestSplitTestPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"single segment", "TestFoo", []string{"TestFoo"}},
		{"two segments", "TestFoo/bar", []string{"TestFoo", "bar"}},
		{"three segments", "TestFoo/bar/baz", []string{"TestFoo", "bar", "baz"}},
		{"consecutive double slash preserved", "TestFoo/https://example.com", []string{"TestFoo", "https://example.com"}},
		{"consecutive triple slash preserved", "TestFoo/a///b", []string{"TestFoo", "a///b"}},
		{"empty string", "", []string{}},
		{"trailing slash", "TestFoo/bar/", []string{"TestFoo", "bar"}},
		{"mixed normal and double slash", "TestSuite/method/https://host/path", []string{"TestSuite", "method", "https://host", "path"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitTestPath(tt.path)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			gotest.Len(t, got, len(tt.want), "splitTestPath(%q) = %v, want %v", tt.path, got, tt.want)
			for i := range got {
				gotest.Equal(t, tt.want[i], got[i], "splitTestPath(%q)[%d]", tt.path, i)
			}
		})
	}
}

func TestBuildTree_DuplicateSuite_PtestPxtest(t *testing.T) {
	// Simulates same-named suite in ptest and pxtest. Go runs both and appends
	// #01 to subtests of the second run.
	input := `{"Action":"run","Package":"example.com/stdlib","Test":"TestUnitTestSuite"}
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
{"Action":"pass","Package":"example.com/stdlib","Elapsed":0.05}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)

	tree := BuildTree(events)
	gotest.Len(t, tree, 1, "packages")
	pkg := tree[0]

	// Should produce 2 separate suite nodes, not 1 merged one.
	gotest.Len(t, pkg.Nodes, 2, "root nodes")

	suite1 := pkg.Nodes[0]
	suite2 := pkg.Nodes[1]

	gotest.Equal(t, KindSuite, suite1.Kind, "suite1 kind")
	gotest.Equal(t, KindSuite, suite2.Kind, "suite2 kind")
	gotest.Equal(t, "Unit", suite1.Display, "suite1 display")
	gotest.Equal(t, "Unit", suite2.Display, "suite2 display")

	// Each suite should have 2 methods (not 4 merged).
	gotest.Len(t, suite1.Children, 2, "suite1 children")
	gotest.Len(t, suite2.Children, 2, "suite2 children")

	// Children of suite2 should NOT have #01 suffix.
	for _, c := range suite2.Children {
		gotest.NotContains(t, c.Name, "#", "suite2 child name still has # suffix")
		gotest.NotContains(t, c.Display, "#", "suite2 child display still has # suffix")
	}

	// suite2 should be marked as variant 2 and external.
	gotest.Equal(t, 2, suite2.Variant, "suite2 variant")
	gotest.True(t, suite2.External, "expected suite2 to be external")
	gotest.False(t, suite1.External, "expected suite1 to not be external")

	// Both should have pass status.
	gotest.Equal(t, StatusPass, suite1.Status, "suite1 status")
	gotest.Equal(t, StatusPass, suite2.Status, "suite2 status")
}

func TestClassify_ParallelMethod(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestMyTestSuite"}
{"Action":"run","Package":"p","Test":"TestMyTestSuite/TestParallelCreate"}
{"Action":"pass","Package":"p","Test":"TestMyTestSuite/TestParallelCreate","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestMyTestSuite","Elapsed":0.02}
{"Action":"pass","Package":"p","Elapsed":0.05}`

	events, _ := ParseEvents(strings.NewReader(input))
	tree := BuildTree(events)

	method := tree[0].Nodes[0].Children[0]
	gotest.Equal(t, KindMethod, method.Kind)
	gotest.Equal(t, "ParallelCreate", method.Display)
}

func TestBuildTree_BenchmarkEvents(t *testing.T) {
	events := []TestEvent{
		{Action: ActionOutput, Package: "p", Test: "BenchmarkFooTestSuite/BenchmarkParse",
			Output: "BenchmarkFooTestSuite/BenchmarkParse-8   \t 1201 \t 985.2 ns/op \t 24 B/op \t 3 allocs/op\n"},
		{Action: ActionBench, Package: "p", Test: "BenchmarkFooTestSuite/BenchmarkParse"},
	}
	pkgs := BuildTree(events)
	leaf := pkgs[0].Nodes[0].Children[0]
	gotest.Equal(t, KindBenchmark, leaf.Kind)
	gotest.Equal(t, StatusPass, leaf.Status)
	gotest.InDelta(t, 985.2, leaf.NsPerOp, 0.001)
	gotest.Equal(t, int64(24), leaf.BytesPerOp)
	gotest.Equal(t, int64(3), leaf.AllocsPerOp)
	stats := CollectStats(pkgs)
	gotest.Equal(t, 1, stats.Benchmarks)
	gotest.Equal(t, 0, stats.Behaviors)
}

func TestBuildTree_BenchmarkOutputSplitAcrossEvents(t *testing.T) {
	// test2json "output" events are not guaranteed line-aligned; under real
	// subprocess pipe timing a bench result line can arrive split mid-token
	// across two consecutive events for the same tagged Test. Metrics must
	// still be recovered by scanning the node's joined output, not just the
	// single event that happens to complete the line.
	events := []TestEvent{
		{Action: ActionOutput, Package: "p", Test: "BenchmarkFoo",
			Output: "BenchmarkFoo-8   \t 12"},
		{Action: ActionOutput, Package: "p", Test: "BenchmarkFoo",
			Output: "01 \t 985.2 ns/op\n"},
		{Action: ActionBench, Package: "p", Test: "BenchmarkFoo"},
	}
	pkgs := BuildTree(events)
	leaf := pkgs[0].Nodes[0]
	gotest.Equal(t, KindBenchmark, leaf.Kind)
	gotest.Equal(t, StatusPass, leaf.Status)
	gotest.Equal(t, 1201, leaf.Iterations)
	gotest.InDelta(t, 985.2, leaf.NsPerOp, 0.001)
}

func TestClassify_TopLevelTestNamedBenchmarkIsNotABenchmark(t *testing.T) {
	// "TestBenchmarkFoo" is a legitimate stdlib test whose name merely
	// starts with "Benchmark" after the "Test" prefix is trimmed. It must
	// resolve through the ordinary test classification, never the bench
	// branch.
	input := `{"Action":"run","Package":"p","Test":"TestBenchmarkFoo"}
{"Action":"pass","Package":"p","Test":"TestBenchmarkFoo","Elapsed":0.01}
{"Action":"pass","Package":"p","Elapsed":0.01}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)
	tree := BuildTree(events)

	node := tree[0].Nodes[0]
	gotest.Equal(t, KindTest, node.Kind)
	gotest.Equal(t, "BenchmarkFoo", node.Display)
}

func TestClassify_NestedBenchmarkingPrefixIsNotABenchmark(t *testing.T) {
	// "Benchmarking_the_new_endpoint" starts with "Benchmark" but continues
	// with a lowercase letter ("ing..."), so it is an ordinary "when/it"
	// style block name, not a Go benchmark identifier.
	input := `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestBar"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/TestBar/Benchmarking_the_new_endpoint"}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestBar/Benchmarking_the_new_endpoint","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite/TestBar","Elapsed":0.01}
{"Action":"pass","Package":"p","Test":"TestFooTestSuite","Elapsed":0.01}
{"Action":"pass","Package":"p","Elapsed":0.01}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)
	tree := BuildTree(events)

	block := tree[0].Nodes[0].Children[0].Children[0]
	gotest.Equal(t, KindBlock, block.Kind)
	gotest.Equal(t, "Benchmarking the new endpoint", block.Display)
}

func TestClassify_NestedBenchmarkName(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestFooTestSuite"}
{"Action":"run","Package":"p","Test":"TestFooTestSuite/BenchmarkParse"}
{"Action":"output","Package":"p","Test":"TestFooTestSuite/BenchmarkParse","Output":"BenchmarkParse-8   \t 100 \t 10.0 ns/op\n"}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)
	tree := BuildTree(events)

	leaf := tree[0].Nodes[0].Children[0]
	gotest.Equal(t, KindBenchmark, leaf.Kind)
	gotest.Equal(t, "Parse", leaf.Display)
}

func TestBuildTree_PackageLevelOutput(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestFoo"}
{"Action":"output","Package":"p","Test":"TestFoo","Output":"=== RUN   TestFoo\n"}
{"Action":"output","Package":"p","Test":"TestFoo","Output":"--- PASS: TestFoo (0.00s)\n"}
{"Action":"pass","Package":"p","Test":"TestFoo","Elapsed":0}
{"Action":"output","Package":"p","Output":"==================\n"}
{"Action":"output","Package":"p","Output":"WARNING: DATA RACE\n"}
{"Action":"output","Package":"p","Output":"Write at 0x00c by goroutine 9:\n"}
{"Action":"output","Package":"p","Output":"==================\n"}
{"Action":"output","Package":"p","Output":"Found 1 data race(s)\n"}
{"Action":"output","Package":"p","Output":"FAIL\tp\t1.0s\n"}
{"Action":"fail","Package":"p","Elapsed":1.0}`

	events, err := ParseEvents(strings.NewReader(input))
	gotest.NoError(t, err)

	tree := BuildTree(events)
	gotest.Len(t, tree, 1, "packages")
	pkg := tree[0]

	// Test node should be present and passed
	gotest.Len(t, pkg.Nodes, 1, "nodes")
	gotest.Equal(t, StatusPass, pkg.Nodes[0].Status)

	// Package should have failed
	gotest.Equal(t, StatusFail, pkg.Status)

	// Package-level diagnostic output should be collected
	gotest.NotEmpty(t, pkg.Output, "expected package-level output")

	combined := strings.Join(pkg.Output, "")
	gotest.Contains(t, combined, "WARNING: DATA RACE")
	gotest.Contains(t, combined, "Found 1 data race(s)")

	// Summary lines should NOT be in package output
	gotest.NotContains(t, combined, "FAIL\tp\t", "package output should not contain summary line")
}
