package gotestspec

import (
	"strings"
	"testing"
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
	if err != nil {
		t.Fatal(err)
	}

	tree := BuildTree(events)
	if len(tree) != 1 {
		t.Fatalf("expected 1 package, got %d", len(tree))
	}
	pkg := tree[0]
	if pkg.Path != "example.com/pkg" {
		t.Errorf("package path = %q", pkg.Path)
	}
	if len(pkg.Nodes) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(pkg.Nodes))
	}

	suite := pkg.Nodes[0]
	if suite.Kind != KindSuite {
		t.Errorf("root kind = %d, want KindSuite", suite.Kind)
	}
	if suite.Display != "UserService" {
		t.Errorf("suite display = %q, want UserService", suite.Display)
	}

	if len(suite.Children) != 1 {
		t.Fatalf("expected 1 method, got %d", len(suite.Children))
	}
	method := suite.Children[0]
	if method.Kind != KindMethod {
		t.Errorf("method kind = %d, want KindMethod", method.Kind)
	}
	if method.Display != "Create" {
		t.Errorf("method display = %q, want Create", method.Display)
	}

	if len(method.Children) != 1 {
		t.Fatalf("expected 1 when block, got %d", len(method.Children))
	}
	when := method.Children[0]
	if when.Kind != KindBlock {
		t.Errorf("when kind = %d, want KindBlock", when.Kind)
	}
	if when.Display != "when email is valid" {
		t.Errorf("when display = %q", when.Display)
	}

	if len(when.Children) != 1 {
		t.Fatalf("expected 1 it block, got %d", len(when.Children))
	}
	it := when.Children[0]
	if it.Kind != KindBlock {
		t.Errorf("it kind = %d, want KindBlock", it.Kind)
	}
	if it.Display != "creates the user" {
		t.Errorf("it display = %q", it.Display)
	}
	if it.Status != StatusPass {
		t.Errorf("it status = %d, want StatusPass", it.Status)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	tree := BuildTree(events)
	pkg := tree[0]
	fixture := pkg.Nodes[0]
	if fixture.Kind != KindFixture {
		t.Errorf("root kind = %d, want KindFixture", fixture.Kind)
	}
	if fixture.Display != "Infra" {
		t.Errorf("fixture display = %q, want Infra", fixture.Display)
	}

	child := fixture.Children[0]
	if child.Kind != KindFixture {
		t.Errorf("child kind = %d, want KindFixture", child.Kind)
	}
	if child.Display != "API" {
		t.Errorf("child display = %q, want API", child.Display)
	}

	suite := child.Children[0]
	if suite.Kind != KindSuite {
		t.Errorf("suite kind = %d, want KindSuite", suite.Kind)
	}
	if suite.Display != "Batch" {
		t.Errorf("suite display = %q, want Batch", suite.Display)
	}

	method := suite.Children[0]
	if method.Kind != KindMethod {
		t.Errorf("method kind = %d, want KindMethod", method.Kind)
	}
	if method.Display != "Dispatch" {
		t.Errorf("method display = %q, want Dispatch", method.Display)
	}
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
	if !suite.Focused {
		t.Error("expected suite to be focused")
	}
	if suite.Display != "PaymentService" {
		t.Errorf("display = %q, want PaymentService", suite.Display)
	}
}

func TestBuildTree_ExcludedSuite(t *testing.T) {
	input := `{"Action":"run","Package":"p","Test":"TestX_BrokenTestSuite"}
{"Action":"skip","Package":"p","Test":"TestX_BrokenTestSuite","Elapsed":0}
{"Action":"pass","Package":"p","Elapsed":0.1}`

	events, _ := ParseEvents(strings.NewReader(input))
	tree := BuildTree(events)

	suite := tree[0].Nodes[0]
	if !suite.Excluded {
		t.Error("expected suite to be excluded")
	}
	if suite.Display != "Broken" {
		t.Errorf("display = %q, want Broken", suite.Display)
	}
	if suite.Status != StatusSkip {
		t.Errorf("status = %d, want StatusSkip", suite.Status)
	}
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

	if stats.Suites != 2 {
		t.Errorf("suites = %d, want 2", stats.Suites)
	}
	if stats.Behaviors != 3 {
		t.Errorf("behaviors = %d, want 3", stats.Behaviors)
	}
	if stats.Tests != 0 {
		t.Errorf("tests = %d, want 0", stats.Tests)
	}
	if stats.Passed != 1 {
		t.Errorf("passed = %d, want 1", stats.Passed)
	}
	if stats.Failed != 1 {
		t.Errorf("failed = %d, want 1", stats.Failed)
	}
	if stats.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", stats.Skipped)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	tree := BuildTree(events)
	pkg := tree[0]
	if len(pkg.Nodes) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(pkg.Nodes))
	}

	test := pkg.Nodes[0]
	if test.Kind != KindTest {
		t.Errorf("root kind = %d, want KindTest", test.Kind)
	}
	if test.Display != "CreateUser" {
		t.Errorf("display = %q, want CreateUser", test.Display)
	}
	if len(test.Children) != 2 {
		t.Fatalf("expected 2 subtests, got %d", len(test.Children))
	}
	if test.Children[0].Kind != KindBlock {
		t.Errorf("subtest kind = %d, want KindBlock", test.Children[0].Kind)
	}
	if test.Children[0].Display != "valid email" {
		t.Errorf("subtest display = %q, want 'valid email'", test.Children[0].Display)
	}
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

	if stats.Suites != 1 {
		t.Errorf("suites = %d, want 1", stats.Suites)
	}
	if stats.Behaviors != 1 {
		t.Errorf("behaviors = %d, want 1", stats.Behaviors)
	}
	if stats.Tests != 2 {
		t.Errorf("tests = %d, want 2", stats.Tests)
	}
	if stats.Passed != 3 {
		t.Errorf("passed = %d, want 3", stats.Passed)
	}
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

	if stats.Suites != 0 {
		t.Errorf("suites = %d, want 0", stats.Suites)
	}
	if stats.Behaviors != 0 {
		t.Errorf("behaviors = %d, want 0", stats.Behaviors)
	}
	if stats.Tests != 2 {
		t.Errorf("tests = %d, want 2", stats.Tests)
	}
	if stats.Passed != 2 {
		t.Errorf("passed = %d, want 2", stats.Passed)
	}
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
			if len(got) != len(tt.want) {
				t.Fatalf("splitTestPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitTestPath(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.want[i])
				}
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
	if err != nil {
		t.Fatal(err)
	}

	tree := BuildTree(events)
	if len(tree) != 1 {
		t.Fatalf("expected 1 package, got %d", len(tree))
	}
	pkg := tree[0]

	// Should produce 2 separate suite nodes, not 1 merged one.
	if len(pkg.Nodes) != 2 {
		t.Fatalf("expected 2 root nodes, got %d", len(pkg.Nodes))
	}

	suite1 := pkg.Nodes[0]
	suite2 := pkg.Nodes[1]

	if suite1.Kind != KindSuite {
		t.Errorf("suite1 kind = %d, want KindSuite", suite1.Kind)
	}
	if suite2.Kind != KindSuite {
		t.Errorf("suite2 kind = %d, want KindSuite", suite2.Kind)
	}
	if suite1.Display != "Unit" {
		t.Errorf("suite1 display = %q, want Unit", suite1.Display)
	}
	if suite2.Display != "Unit" {
		t.Errorf("suite2 display = %q, want Unit", suite2.Display)
	}

	// Each suite should have 2 methods (not 4 merged).
	if len(suite1.Children) != 2 {
		t.Fatalf("suite1 expected 2 children, got %d", len(suite1.Children))
	}
	if len(suite2.Children) != 2 {
		t.Fatalf("suite2 expected 2 children, got %d", len(suite2.Children))
	}

	// Children of suite2 should NOT have #01 suffix.
	for _, c := range suite2.Children {
		if strings.Contains(c.Name, "#") {
			t.Errorf("suite2 child %q still has # suffix", c.Name)
		}
		if strings.Contains(c.Display, "#") {
			t.Errorf("suite2 child display %q still has # suffix", c.Display)
		}
	}

	// suite2 should be marked as variant 2.
	if suite2.Variant != 2 {
		t.Errorf("suite2 variant = %d, want 2", suite2.Variant)
	}

	// Both should have pass status.
	if suite1.Status != StatusPass {
		t.Errorf("suite1 status = %d, want StatusPass", suite1.Status)
	}
	if suite2.Status != StatusPass {
		t.Errorf("suite2 status = %d, want StatusPass", suite2.Status)
	}
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
	if method.Kind != KindMethod {
		t.Errorf("kind = %d, want KindMethod", method.Kind)
	}
	if method.Display != "ParallelCreate" {
		t.Errorf("display = %q, want ParallelCreate", method.Display)
	}
}
