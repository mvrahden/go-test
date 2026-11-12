package cli_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLISummaryTestSuite: what gotest summary's first line says about a run.
type CLISummaryTestSuite struct{}

func (s *CLISummaryTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second, Parallel: true}
}

func (s *CLISummaryTestSuite) TestTheHeadline(t *gotest.T) {
	t.When("gotest summary runs a package whose tests all pass", func(w *gotest.T) {
		w.It("the Markdown headline says All and the count of tests", func(it *gotest.T) {
			md, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--format=md", "../testdata/passing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, md.Exit, "stdout: %s\nstderr: %s", md.Stdout, md.Stderr)
			gotest.Regexp(it, `^### All \d+ tests? passed \(`, md.Stdout)
		})
	})

	t.When("gotest summary runs a package with passed and skipped tests", func(w *gotest.T) {
		w.It("the headline counts the passed tests and names the skipped ones", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "../testdata/summarymix/")
			gotest.NoError(it, err)
			gotest.Regexp(it, `^2 tests passed, 2 skipped \(`, run.Stdout, "stderr: %s", run.Stderr)
		})
		w.It("the Markdown headline does not say All", func(it *gotest.T) {
			md, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--format=md", "../testdata/summarymix/")
			gotest.NoError(it, err)
			gotest.Regexp(it, `^### 2 tests passed, 2 skipped \(`, md.Stdout, "stderr: %s", md.Stderr)
			gotest.NotContains(it, md.Stdout, "All")
		})
		w.It("the GitHub step summary carries the Markdown headline", func(it *gotest.T) {
			step := filepath.Join(it.TempDir(), "step.md")
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GITHUB_STEP_SUMMARY=" + step}, "summary", "--github", "--no-color", "../testdata/summarymix/")
			gotest.NoError(it, err)
			file, err := os.ReadFile(step)
			gotest.NoError(it, err, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `^### 2 tests passed, 2 skipped \(`, string(file))
		})
	})

	t.When("gotest summary runs a package whose tests are all skipped", func(w *gotest.T) {
		w.It("the headline says no tests passed, names the skipped ones and exits 0", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"SUMMARYMIX=skips"}, "summary", "--no-color", "../testdata/summarymix/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `^No tests passed: 4 skipped \(`, run.Stdout)
		})
	})

	t.When("gotest summary runs a package with failed, passed and skipped tests", func(w *gotest.T) {
		w.It("the headline opens with the failed count, then the passed and the skipped", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"SUMMARYMIX=fail"}, "summary", "--no-color", "../testdata/summarymix/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `^1 test failed, 1 passed, 2 skipped\n`, run.Stdout)
		})
	})
}

func (s *CLISummaryTestSuite) TestTheStdlibNote(t *gotest.T) {
	t.When("gotest summary writes Markdown for a package that holds a suite and a stdlib test", func(w *gotest.T) {
		root, err := testkit.RepoRoot()
		gotest.NoError(w, err)
		dir := w.TempDir()
		gotest.NoError(w, testkit.StageModule(root, filepath.Join("..", "testdata", "climixedtestify"), dir))
		md, err := testkit.RunCLI(w.Context(), dir, "summary", "--format=md", "./mixed/")
		gotest.NoError(w, err)

		w.It("the note of the stdlib tests not run follows the headline", func(it *gotest.T) {
			gotest.Equal(it, 0, md.Exit, "stdout: %s\nstderr: %s", md.Stdout, md.Stderr)
			gotest.Contains(it, md.Stdout, "> **Note:** 1 stdlib test")
			gotest.Contains(it, md.Stdout, "climixedtestifymod/mixed not run")
			// A package without stdlib tests gets no note.
			plain, err := testkit.RunCLI(it.Context(), ".", "summary", "--format=md", "../testdata/passing/")
			gotest.NoError(it, err)
			gotest.NotContains(it, plain.Stdout, "**Note:**")
		})
		w.It("the headline does not say All", func(it *gotest.T) {
			gotest.NotContains(it, md.Stdout, "### All")
		})
		w.It("the GitHub step summary carries the note", func(it *gotest.T) {
			step := filepath.Join(it.TempDir(), "step.md")
			run, err := testkit.RunCLIWithEnv(it.Context(), dir, []string{"GITHUB_STEP_SUMMARY=" + step}, "summary", "--github", "--no-color", "./mixed/")
			gotest.NoError(it, err)
			file, err := os.ReadFile(step)
			gotest.NoError(it, err, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, string(file), "> **Note:** 1 stdlib test")
		})
	})
}
