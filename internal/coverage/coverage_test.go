package coverage

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestExtractTypeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"UserServiceTestSuite", "UserService"},
		{"F_UserServiceTestSuite", "UserService"},
		{"X_UserServiceTestSuite", "UserService"},
		{"BatchTestSuiteParallel", "Batch"},
		{"F_BatchTestSuiteParallel", "Batch"},
		{"NotASuite", ""},
		{"TestSuite", ""},
		{"FooBar", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractTypeName(tt.input)
			if got != tt.want {
				t.Errorf("extractTypeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindMatchingTestMethod(t *testing.T) {
	methods := []string{"TestCreate", "TestGetByID", "F_TestDelete", "TestParallelFetch"}

	tests := []struct {
		method string
		want   string
	}{
		{"Create", "TestCreate"},
		{"GetByID", "TestGetByID"},
		{"Delete", "F_TestDelete"},
		{"Fetch", "TestParallelFetch"},
		{"Update", ""},
		{"Nonexistent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := findMatchingTestMethod(tt.method, methods)
			if got != tt.want {
				t.Errorf("findMatchingTestMethod(%q) = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestFindMatchingSuite(t *testing.T) {
	suites := []testSuite{
		{name: "UserServiceTestSuite", typeName: "UserService"},
		{name: "OrderTestSuite", typeName: "Order"},
	}

	t.Run("matches by type name", func(t *testing.T) {
		got := findMatchingSuite("UserService", suites)
		if got == nil || got.name != "UserServiceTestSuite" {
			t.Errorf("expected UserServiceTestSuite, got %v", got)
		}
	})

	t.Run("returns nil for no match", func(t *testing.T) {
		got := findMatchingSuite("Payment", suites)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestFindTestSuites(t *testing.T) {
	dir := t.TempDir()
	src := `package foo

type UserServiceTestSuite struct{}

func (s *UserServiceTestSuite) TestCreate()      {}
func (s *UserServiceTestSuite) F_TestDelete()    {}
func (s *UserServiceTestSuite) X_TestSkipped()   {}
func (s *UserServiceTestSuite) TestParallelFetch() {}
func (s *UserServiceTestSuite) BeforeEach()      {}
`
	if err := os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	suites := findTestSuites(dir)
	if len(suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(suites))
	}

	s := suites[0]
	if s.name != "UserServiceTestSuite" {
		t.Errorf("name = %q, want UserServiceTestSuite", s.name)
	}
	if s.typeName != "UserService" {
		t.Errorf("typeName = %q, want UserService", s.typeName)
	}

	sort.Strings(s.methods)
	want := []string{"F_TestDelete", "TestCreate", "TestParallelFetch", "X_TestSkipped"}
	if strings.Join(s.methods, ",") != strings.Join(want, ",") {
		t.Errorf("methods = %v, want %v", s.methods, want)
	}
}

func TestFindTestSuitesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	src := `package foo

type NotASuite struct{}

func (s *NotASuite) DoStuff() {}
`
	if err := os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	suites := findTestSuites(dir)
	if len(suites) != 0 {
		t.Errorf("expected 0 suites, got %d", len(suites))
	}
}

func TestRender(t *testing.T) {
	report := &Report{
		Packages: []PackageReport{
			{
				Path: "example/user",
				Types: []TypeReport{
					{
						Name: "UserService",
						Methods: []MethodReport{
							{Name: "Create", Covered: true, TestMethod: "TestCreate"},
							{Name: "Delete", Covered: false},
							{Name: "Get", Covered: true, TestMethod: "TestGet"},
						},
					},
				},
			},
		},
		Total:   3,
		Covered: 2,
	}

	var buf bytes.Buffer
	Render(&buf, report)
	out := buf.String()

	if !strings.Contains(out, "UserService: 2/3 methods covered (66%)") {
		t.Errorf("expected type summary, got:\n%s", out)
	}
	if !strings.Contains(out, "✓ Create") {
		t.Errorf("expected covered method Create, got:\n%s", out)
	}
	if !strings.Contains(out, "✗ Delete") {
		t.Errorf("expected uncovered method Delete, got:\n%s", out)
	}
	if !strings.Contains(out, "Overall: 2/3 methods covered (66%)") {
		t.Errorf("expected overall summary, got:\n%s", out)
	}
}
