package lint_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterDeclarationsTestSuite: a suite or fixture is one type declaration of
// its own, composed through named fields, and its hooks are the harness's to
// call; lint reports each departure where it is written.
type LinterDeclarationsTestSuite struct{}

func (s *LinterDeclarationsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *LinterDeclarationsTestSuite) TestTypeBlocks(t *gotest.T) {
	t.When("a suite is declared inside a type block", func(w *gotest.T) {
		w.It("lint reports a suite declared in a type block at its declaration", func(it *gotest.T) {
			gotest.Contains(it, declFindings(it, "../testdata/lintdeclgrouped/"),
				"suite-declaration grouped_suite_test.go:9:2: suite GroupedTestSuite is declared inside a type block; give it its own type declaration")
		})
		w.It("lint -fix moves it into its own declaration, doc comment kept, and the package then generates", func(it *gotest.T) {
			root, err := testkit.RepoRoot()
			gotest.NoError(it, err)
			dir, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "lintdeclgrouped"), it.TempDir(), "lintdeclmod")
			gotest.NoError(it, err)

			fix, err := testkit.RunCLI(it.Context(), dir, "lint", "-fix", ".")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, fix.Exit, "stderr: %s", fix.Stderr)
			src, err := os.ReadFile(filepath.Join(dir, "grouped_suite_test.go"))
			gotest.NoError(it, err)
			gotest.Regexp(it, `(?m)^type \(\n\tcounter int\n\)\n\n// GroupedTestSuite is declared inside a type block\.\ntype GroupedTestSuite struct \{\n\tn counter\n\}\n`, string(src))

			run, err := testkit.RunCLI(it.Context(), dir, "-json", ".")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestGroupedTestSuite/TestCounts"), "stdout: %s", run.Stdout)
		})
	})
}

func (s *LinterDeclarationsTestSuite) TestEmbedding(t *gotest.T) {
	t.When("a suite embeds a fixture", func(w *gotest.T) {
		w.It("lint reports an embedded fixture at the field with the alternative", func(it *gotest.T) {
			gotest.Contains(it, declFindings(it, "../testdata/lintdeclembed/"),
				"no-embedding embed_suite_test.go:14:3: suite ChildTestSuite embeds fixture StoreFixture; fixtures compose through a named pointer field")
		})
	})
}

func (s *LinterDeclarationsTestSuite) TestHookCalls(t *gotest.T) {
	t.When("a suite method calls a fixture's lifecycle hook", func(w *gotest.T) {
		w.It("lint reports a direct call of a fixture's lifecycle hook at the call", func(it *gotest.T) {
			gotest.Contains(it, declFindings(it, "../testdata/lintdeclhookcall/"),
				"fixture-hook-call hookcall_suite_test.go:23:20: BeforeEach is called by the harness; calling it here runs it twice")
		})
	})
}

func (s *LinterDeclarationsTestSuite) TestOwnConnect(t *gotest.T) {
	t.When("a shared fixture's BeforeAll or AfterAll calls its own Hydrate or Dehydrate", func(w *gotest.T) {
		w.It("lint leaves the call unreported, since the process that owns the fixture runs neither", func(it *gotest.T) {
			for _, f := range declFindings(it, "../testdata/lintdeclhookcall/") {
				gotest.NotContains(it, f, "shared.go:")
			}
		})
	})
}

// declFindings lints pkg with -json and returns each gotest finding as
// "<rule> <file>:<line>:<col>: <message>".
func declFindings(t *gotest.T, pkg string) []string {
	run, err := testkit.RunCLI(t.Context(), ".", "lint", "-json", pkg)
	gotest.NoError(t, err)
	var byPkg map[string]map[string][]struct {
		Posn     string `json:"posn"`
		Message  string `json:"message"`
		Category string `json:"category"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &byPkg), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	var got []string
	for _, analyzers := range byPkg {
		for _, d := range analyzers["gotestlint"] {
			got = append(got, d.Category+" "+filepath.Base(d.Posn)+": "+d.Message)
		}
	}
	return got
}
