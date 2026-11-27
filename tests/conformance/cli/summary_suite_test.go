package cli_test

import (
	"os"
	"path/filepath"
	"strings"
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

func (s *CLISummaryTestSuite) TestAnnotations(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)

	t.When("gotest summary --github runs a package that fails to build", func(w *gotest.T) {
		w.It("annotates the compiler's error at the file's path from the working directory", func(it *gotest.T) {
			dir := it.TempDir()
			_, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "annotbroken"), dir, "annotmod")
			gotest.NoError(it, err)
			run, err := testkit.RunCLI(it.Context(), dir, "summary", "--github", "--no-color", "./annotbroken/")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `(?m)^::error file=annotbroken/annotbroken_suite_test\.go,line=\d+,`, run.Stdout)
			gotest.NotContains(it, run.Stdout, "file=annotbroken/annotbroken/")
		})
	})

	t.When("gotest summary --github runs a package with a failed test", func(w *gotest.T) {
		w.It("annotates the failure at the test file in its package's directory", func(it *gotest.T) {
			dir := it.TempDir()
			_, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "failing"), dir, "annotmod")
			gotest.NoError(it, err)
			run, err := testkit.RunCLI(it.Context(), dir, "summary", "--github", "--no-color", "./failing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `(?m)^::error file=failing/failing_suite_test\.go,line=9,`, run.Stdout)
		})
	})
}

// runFailedHeadline opens the summary of the assertcount package's --ci run.
const runFailedHeadline = `Run failed with exit 1, no test failed: 12 passed, 1 skipped, 5 vacuous \(`

func (s *CLISummaryTestSuite) TestARunThatFailedWithNoFailedTest(t *gotest.T) {
	t.When("gotest summary --ci runs a package whose only problem is tests that passed without an assertion", func(w *gotest.T) {
		w.It("the headline opens with Run failed with exit 1, says no test failed and counts the passed, skipped and vacuous tests", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--ci", "--no-color", vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `^`+runFailedHeadline, run.Stdout)
			gotest.Contains(it, run.Stdout, "VACUOUS  ")
		})
		w.It("the Markdown headline and the GitHub step summary open so too", func(it *gotest.T) {
			md, err := testkit.RunCLI(it.Context(), ".", "summary", "--ci", "--no-color", "--format=md", vacuousPkg)
			gotest.NoError(it, err)
			gotest.Regexp(it, `^### `+runFailedHeadline, md.Stdout, "stderr: %s", md.Stderr)
			step := filepath.Join(it.TempDir(), "step.md")
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GITHUB_STEP_SUMMARY=" + step}, "summary", "--github", "--ci", "--no-color", vacuousPkg)
			gotest.NoError(it, err)
			file, err := os.ReadFile(step)
			gotest.NoError(it, err, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `^### `+runFailedHeadline, string(file))
		})
		w.It("the summary of the run's saved stream opens so too", func(it *gotest.T) {
			events := filepath.Join(it.TempDir(), "events.json")
			live, err := testkit.RunCLI(it.Context(), ".", "summary", "--ci", "--no-color", "--events-file="+events, vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, live.Exit, "stderr: %s", live.Stderr)
			replay, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--input="+events)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, replay.Exit, "stderr: %s", replay.Stderr)
			gotest.Regexp(it, `^`+runFailedHeadline, replay.Stdout)
		})
	})

	t.When("gotest summary --min=100 runs a package whose tests pass and cover nothing", func(w *gotest.T) {
		w.It("the headline opens with Run failed with exit 1, says no test failed and counts the passed test", func(it *gotest.T) {
			events := filepath.Join(it.TempDir(), "events.json")
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--min=100", "--events-file="+events, "../testdata/passing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `^Run failed with exit 1, no test failed: 1 passed \(`, run.Stdout)
			gotest.Contains(it, run.Stderr, "FAIL: 0.0% coverage (minimum 100%)")
			// The saved stream replays as the run ended: the limit is in its exit code.
			replay, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--input="+events)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, replay.Exit, "stderr: %s", replay.Stderr)
			gotest.Regexp(it, `^Run failed with exit 1, no test failed: 1 passed \(`, replay.Stdout)
		})
		w.It("the Markdown headline does not say All", func(it *gotest.T) {
			md, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--format=md", "--min=100", "../testdata/passing/")
			gotest.NoError(it, err)
			gotest.Regexp(it, `^### Run failed with exit 1, no test failed: 1 passed \(`, md.Stdout, "stderr: %s", md.Stderr)
			gotest.NotContains(it, md.Stdout, "All")
		})
	})
}

func (s *CLISummaryTestSuite) TestTheRunFailureAnnotation(t *gotest.T) {
	t.When("gotest summary --github --ci runs a package whose only problem is tests that passed without an assertion", func(w *gotest.T) {
		w.It("writes one error annotation without a file, naming exit 1 and the 5 tests that passed without an assertion", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--github", "--ci", "--no-color", vacuousPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, 1, strings.Count(run.Stdout, "::error "), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "\n::error title=gotest run failed::exit 1 and no test failed: 5 tests passed without an assertion, which fails a run in CI mode\n")
		})
	})

	t.When("gotest summary --github --min=100 runs a package whose tests pass and cover nothing", func(w *gotest.T) {
		w.It("writes one error annotation without a file, naming exit 1 and the run's own output", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--github", "--no-color", "--min=100", "../testdata/passing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, 1, strings.Count(run.Stdout, "::error "), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "\n::error title=gotest run failed::exit 1 and no test failed: the run's own output names what failed it\n")
		})
	})
}
