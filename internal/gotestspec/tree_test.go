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
	if method.Display != "Create" {
		t.Errorf("display = %q, want Create", method.Display)
	}
}
