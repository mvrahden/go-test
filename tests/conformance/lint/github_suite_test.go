package lint_test

import (
	"strings"
	"unicode/utf8"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterGitHubTestSuite: lint --github writes each finding as a workflow
// command GitHub reads back as it was meant.
type LinterGitHubTestSuite struct{}

func (s *LinterGitHubTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *LinterGitHubTestSuite) TestAnnotationEscaping(t *gotest.T) {
	t.When("a finding's message and file carry characters a workflow command escapes", func(w *gotest.T) {
		run, err := testkit.RunCLI(w.Context(), ".", "lint", "--github", "../testdata/lintgh/")
		gotest.NoError(w, err)

		w.It("lint --github escapes the annotation's message and properties", func(it *gotest.T) {
			gotest.Equal(it, 3, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `^::error file=[^,:]*[/\\]lintgh[/\\]fixture%2Cshared\.go,line=8,col=6,title=generation-rejects::`, run.Stdout)
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
