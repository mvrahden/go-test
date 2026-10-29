package migrate_test

import (
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// MigrateFileTestSuite covers the file-level API: suite renaming, what a
// testify suite reports, and the transformed output pinned by snapshot.
type MigrateFileTestSuite struct {
	Mod *gotestcli.TestifyModuleSharedFixture
}

func (s *MigrateFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 90 * time.Second}
}

type migrateFileCtx struct{}

func (s *MigrateFileTestSuite) BeforeEach(t *gotest.T) *migrateFileCtx { return &migrateFileCtx{} }

// transformBasicInput migrates a copy of testdata/basic and returns the result.
func (s *MigrateFileTestSuite) transformBasicInput(t *gotest.T) (string, []migrate.MigrateResult) {
	src, err := os.ReadFile(filepath.Join("testdata", "basic", "input_test.go"))
	gotest.NoError(t, err)
	run := migrateModule(t, s.Mod, migrate.Options{}, map[string]string{"basic/input_test.go": string(src)}, "basic")
	return run.files["basic/input_test.go"], run.results
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
		{Desc: "FooBar", expected: "FooBarTestSuite"},    // no "Suite" suffix
		{Desc: "thingSuite", expected: "ThingTestSuite"}, // gotest runs only exported suites
	}) {
		gotest.Equal(sub, tc.expected, migrate.DeriveNewName(tc.Desc))
	}
}

func (s *MigrateFileTestSuite) TestTokenStarts(t *gotest.T, _ *migrateFileCtx) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		eol  string
	}{
		{"LF line endings", "\n"},
		{"CRLF line endings, which the scanner drops from a literal", "\r\n"},
	}) {
		src := strings.ReplaceAll("package p\n\nvar x = f(`a\nb\nc\n`, y) /* d\ne */\n", "\n", tc.eol)
		gotest.Equal(t, map[int]int{4: 3, 5: 3, 6: 3, 7: 6}, migrate.TokenStarts([]byte(src)))
	}
}

func (s *MigrateFileTestSuite) TestResults(t *gotest.T, _ *migrateFileCtx) {
	_, results := s.transformBasicInput(t)

	gotest.Len(t, results, 1)
	gotest.Equal(t, "UserSuite", results[0].OldName)
	gotest.Equal(t, "UserTestSuite", results[0].NewName)
	gotest.Zero(t, results[0].Markers)
}

func (s *MigrateFileTestSuite) TestTransformFile(t *gotest.T, _ *migrateFileCtx) {
	got, _ := s.transformBasicInput(t)

	// Re-format for consistent whitespace.
	gotBytes, err := format.Source([]byte(got))
	gotest.NoError(t, err, "failed to gofmt result: %v\n\nraw output:\n%s", err, got)

	gotest.MatchSnapshot(t, string(gotBytes))
}
