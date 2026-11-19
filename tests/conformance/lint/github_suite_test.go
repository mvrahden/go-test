package lint_test

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterGitHubTestSuite: lint --github writes each finding as a workflow
// command GitHub reads back as it was meant.
type LinterGitHubTestSuite struct{}

func (s *LinterGitHubTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *LinterGitHubTestSuite) TestAnnotationEscaping(t *gotest.T) {
	t.When("a finding's message and file carry characters a workflow command escapes", func(w *gotest.T) {
		run, err := testkit.RunCLI(w.Context(), ".", "lint", "--github", "../testdata/lintgh/")
		gotest.NoError(w, err)

		w.It("lint --github escapes the annotation's message and properties", func(it *gotest.T) {
			gotest.Equal(it, 3, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `^::error file=[^,:]*[/\\]lintgh[/\\]fixture%2Cshared\.go,line=9,col=2,title=generation-rejects::`, run.Stdout)
			gotest.Contains(it, run.Stdout, `::shared fixture "TagSharedFixture": transfer field "Ch" has non-JSON-serializable type struct{C chan int "x:\"100%25, gh:éé`)
			gotest.NotContains(it, run.Stdout, "100%,")
		})
		w.It("a long annotation is cut on a rune boundary", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(run.Stdout, "\n"), "stdout: %s", run.Stdout)
			gotest.True(it, utf8.ValidString(run.Stdout), "stdout: %q", run.Stdout)
			gotest.Regexp(it, `é\.\.\.\n$`, run.Stdout)
		})
	})
}

func (s *LinterGitHubTestSuite) TestWorkspacePaths(t *gotest.T) {
	t.When("lint --github runs in a subdirectory of the workspace", func(w *gotest.T) {
		w.It("annotation paths are relative to the workspace", func(it *gotest.T) {
			root, err := testkit.RepoRoot()
			gotest.NoError(it, err)
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GITHUB_WORKSPACE=" + root}, "lint", "--github", "../testdata/lintgh/")
			gotest.NoError(it, err)
			gotest.Equal(it, 3, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `^::error file=tests/conformance/testdata/lintgh/fixture%2Cshared\.go,line=9,col=2,`, run.Stdout)
		})
	})
}

func (s *LinterGitHubTestSuite) TestSwitchedOff(t *gotest.T) {
	t.When("lint runs with --github=false inside GitHub Actions", func(w *gotest.T) {
		w.It("lint prints the plain report and no annotation", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GITHUB_ACTIONS=true", "GITHUB_STEP_SUMMARY="}, "lint", "--github=false", "../testdata/lintgh/")
			gotest.NoError(it, err)
			gotest.Equal(it, 3, run.Exit, "stderr: %s", run.Stderr)
			gotest.NotContains(it, run.Stdout, "::error")
			gotest.Regexp(it, `fixture,shared\.go:9:2: shared fixture "TagSharedFixture"`, run.Stderr)
		})
	})
	t.When("--github is given a value that is no boolean", func(w *gotest.T) {
		w.It("lint refuses it with exit 2, naming the value", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "lint", "--github=maybe", "../testdata/lintgh/")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit)
			gotest.Equal(it, "FAIL: invalid value \"maybe\" for flag --github\n", run.Stderr)
		})
	})
}
