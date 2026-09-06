package migrate_test

import (
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// MigrateFileTestSuite covers the file-level API: suite renaming, the analysis
// plan a testify suite yields, and the transformed output pinned by snapshot.
type MigrateFileTestSuite struct{}

func (s *MigrateFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type migrateFileCtx struct{}

func (s *MigrateFileTestSuite) BeforeEach(t *gotest.T) *migrateFileCtx { return &migrateFileCtx{} }

// transformBasicInput migrates testdata/basic and returns the formatted result.
func transformBasicInput(t *gotest.T) string {
	fset := token.NewFileSet()
	inputPath := filepath.Join("testdata", "basic", "input_test.go")
	f, err := parser.ParseFile(fset, inputPath, nil, parser.ParseComments)
	gotest.NoError(t, err, "failed to parse input: %v", err)
	plan := migrate.AnalyzeFile(f)
	gotest.NotEmpty(t, plan.Suites, "no suites detected")
	migrate.TransformFile(fset, f, plan)
	var buf strings.Builder
	gotest.NoError(t, format.Node(&buf, fset, f), "failed to format transformed AST")
	return buf.String()
}

func (s *MigrateFileTestSuite) TestDeriveNewName(t *gotest.T, _ *migrateFileCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		expected string
	}{
		{Desc: "UserSuite", expected: "UserTestSuite"},
		{Desc: "OrderSuite", expected: "OrderTestSuite"},
		{Desc: "UserTestSuite", expected: "UserTestSuite"}, // already ends with TestSuite
		{Desc: "MySuite", expected: "MyTestSuite"},
		{Desc: "FooBar", expected: "FooBarTestSuite"}, // no "Suite" suffix
	}) {
		gotest.Equal(sub, tc.expected, migrate.DeriveNewName(tc.Desc))
	}
}

func (s *MigrateFileTestSuite) TestAnalyzeFile(t *gotest.T, _ *migrateFileCtx) {
	fset := token.NewFileSet()
	inputPath := filepath.Join("testdata", "basic", "input_test.go")
	f, err := parser.ParseFile(fset, inputPath, nil, parser.ParseComments)
	gotest.NoError(t, err, "failed to parse input: %v", err)

	plan := migrate.AnalyzeFile(f)

	gotest.Len(t, plan.Suites, 1)
	suite := plan.Suites[0]
	gotest.Equal(t, "UserSuite", suite.OldName)
	gotest.Equal(t, "UserTestSuite", suite.NewName)
	gotest.Equal(t, "SetupSuite", suite.SetupSuite)
	gotest.Equal(t, "TearDownSuite", suite.TearDownSuite)
	gotest.Equal(t, "SetupTest", suite.SetupTest)
	gotest.Equal(t, "TearDownTest", suite.TearDownTest)
	gotest.Len(t, suite.TestMethods, 2)
	gotest.Equal(t, "TestUserSuite", suite.RunnerFunc)
}

func (s *MigrateFileTestSuite) TestTransformFile(t *gotest.T, _ *migrateFileCtx) {
	got := transformBasicInput(t)

	// Re-format for consistent whitespace.
	gotBytes, err := format.Source([]byte(got))
	gotest.NoError(t, err, "failed to gofmt result: %v\n\nraw output:\n%s", err, got)

	gotest.MatchSnapshot(t, string(gotBytes))
}
