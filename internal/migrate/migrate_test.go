package migrate //nolint:stdlib-test

import (
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
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
			gotest.Equal(t, tt.expected, got)
		})
	}
}

func TestAnalyzeFile(t *testing.T) {
	fset := token.NewFileSet()
	inputPath := filepath.Join("testdata", "basic", "input_test.go")
	f, err := parser.ParseFile(fset, inputPath, nil, parser.ParseComments)
	gotest.NoError(t, err, "failed to parse input: %v", err)

	plan := AnalyzeFile(f)

	gotest.Len(t, plan.Suites, 1)

	s := plan.Suites[0]
	gotest.Equal(t, "UserSuite", s.OldName)
	gotest.Equal(t, "UserTestSuite", s.NewName)
	gotest.Equal(t, "SetupSuite", s.SetupSuite)
	gotest.Equal(t, "TearDownSuite", s.TearDownSuite)
	gotest.Equal(t, "SetupTest", s.SetupTest)
	gotest.Equal(t, "TearDownTest", s.TearDownTest)
	gotest.Len(t, s.TestMethods, 2)
	gotest.Equal(t, "TestUserSuite", s.RunnerFunc)
}

func TestTransformFile(t *testing.T) {
	fset := token.NewFileSet()
	inputPath := filepath.Join("testdata", "basic", "input_test.go")
	f, err := parser.ParseFile(fset, inputPath, nil, parser.ParseComments)
	gotest.NoError(t, err, "failed to parse input: %v", err)

	plan := AnalyzeFile(f)
	gotest.NotEmpty(t, plan.Suites, "no suites detected")

	TransformFile(fset, f, plan)

	// Format the result
	var buf strings.Builder
	err = format.Node(&buf, fset, f)
	gotest.NoError(t, err, "failed to format transformed AST")
	got := buf.String()

	// Re-format for consistent whitespace
	gotBytes, err := format.Source([]byte(got))
	gotest.NoError(t, err, "failed to gofmt result: %v\n\nraw output:\n%s", err, got)

	gotest.MatchSnapshot(t, string(gotBytes))
}
