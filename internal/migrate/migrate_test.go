package migrate

import (
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveNewName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"UserSuite", "UserTestSuite"},
		{"OrderSuite", "OrderTestSuite"},
		{"UserTestSuite", "UserTestSuite"}, // already ends with TestSuite
		{"MySuite", "MyTestSuite"},
		{"FooBar", "FooBarTestSuite"}, // no "Suite" suffix
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DeriveNewName(tt.input)
			if got != tt.expected {
				t.Errorf("DeriveNewName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestAnalyzeFile(t *testing.T) {
	fset := token.NewFileSet()
	inputPath := filepath.Join("testdata", "basic", "input_test.go")
	f, err := parser.ParseFile(fset, inputPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse input: %v", err)
	}

	plan := AnalyzeFile(f)

	if len(plan.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(plan.Suites))
	}

	s := plan.Suites[0]
	if s.OldName != "UserSuite" {
		t.Errorf("OldName = %q, want %q", s.OldName, "UserSuite")
	}
	if s.NewName != "UserTestSuite" {
		t.Errorf("NewName = %q, want %q", s.NewName, "UserTestSuite")
	}
	if s.SetupSuite != "SetupSuite" {
		t.Errorf("SetupSuite = %q, want %q", s.SetupSuite, "SetupSuite")
	}
	if s.TearDownSuite != "TearDownSuite" {
		t.Errorf("TearDownSuite = %q, want %q", s.TearDownSuite, "TearDownSuite")
	}
	if s.SetupTest != "SetupTest" {
		t.Errorf("SetupTest = %q, want %q", s.SetupTest, "SetupTest")
	}
	if s.TearDownTest != "TearDownTest" {
		t.Errorf("TearDownTest = %q, want %q", s.TearDownTest, "TearDownTest")
	}
	if len(s.TestMethods) != 2 {
		t.Errorf("expected 2 test methods, got %d: %v", len(s.TestMethods), s.TestMethods)
	}
	if s.RunnerFunc != "TestUserSuite" {
		t.Errorf("RunnerFunc = %q, want %q", s.RunnerFunc, "TestUserSuite")
	}
}

func TestTransformFile(t *testing.T) {
	fset := token.NewFileSet()
	inputPath := filepath.Join("testdata", "basic", "input_test.go")
	f, err := parser.ParseFile(fset, inputPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse input: %v", err)
	}

	plan := AnalyzeFile(f)
	if len(plan.Suites) == 0 {
		t.Fatal("no suites detected")
	}

	TransformFile(fset, f, plan)

	// Format the result
	var buf strings.Builder
	if err := format.Node(&buf, fset, f); err != nil {
		t.Fatalf("failed to format transformed AST: %v", err)
	}
	got := buf.String()

	// Re-format for consistent whitespace
	gotBytes, err := format.Source([]byte(got))
	if err != nil {
		t.Fatalf("failed to gofmt result: %v\n\nraw output:\n%s", err, got)
	}

	expectedPath := filepath.Join("testdata", "basic", "expected_test.go")
	expectedRaw, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read expected file: %v", err)
	}
	expectedBytes, err := format.Source(expectedRaw)
	if err != nil {
		t.Fatalf("failed to gofmt expected: %v", err)
	}

	if string(gotBytes) != string(expectedBytes) {
		t.Errorf("transformed output does not match expected.\n\n--- GOT ---\n%s\n\n--- EXPECTED ---\n%s", string(gotBytes), string(expectedBytes))
	}
}
