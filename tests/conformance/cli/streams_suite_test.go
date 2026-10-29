package cli_test

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIStreamsTestSuite: a machine-readable stdout holds nothing but the
// document asked for, color follows the terminal, and two output modes
// cannot be asked for at once.
type CLIStreamsTestSuite struct{}

func (s *CLIStreamsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

type streamRow struct {
	Desc string
	env  []string
	args []string
	exit int
}

func (s *CLIStreamsTestSuite) TestJSONOutputsStayPure(t *gotest.T) {
	t.When("a JSON document is asked for", func(w *gotest.T) {
		w.It("prints one JSON document with no color codes or annotations, inside GitHub Actions too", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []streamRow{
				{Desc: "summary --format json inside GitHub Actions carries no annotations", env: []string{"GITHUB_ACTIONS=true"}, args: []string{"summary", "--format", "json", "../testdata/failing/"}, exit: 1},
				{Desc: "summary --format json carries no color codes", args: []string{"summary", "--format", "json", "../testdata/passing/"}, exit: 0},
				{Desc: "spec --format json is one document", args: []string{"spec", "--format", "json", "../testdata/passing/"}, exit: 0},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", row.env, row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stderr: %s", run.Stderr)
				gotest.NotContains(sub, run.Stdout, "\x1b[")
				gotest.NotContains(sub, run.Stdout, "::error")
				var doc any
				gotest.NoError(sub, json.Unmarshal([]byte(run.Stdout), &doc), "stdout is not one JSON document:\n%s", run.Stdout)
			}
		})
	})

	t.When("the go test -json stream is asked for", func(w *gotest.T) {
		w.It("holds JSON lines only, inside GitHub Actions too", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), ".", []string{"GITHUB_ACTIONS=true"}, "-json", "../testdata/failing/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			for _, line := range strings.Split(strings.TrimSpace(run.Stdout), "\n") {
				gotest.Regexp(it, `^\{`, line)
			}
		})
	})
}

func (s *CLIStreamsTestSuite) TestColorFollowsTheTerminal(t *gotest.T) {
	t.When("stdout is a pipe", func(w *gotest.T) {
		w.It("prints no color codes, even with --no-color=false", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []streamRow{
				{Desc: "summary prints no color codes", args: []string{"summary", "../testdata/passing/"}, exit: 0},
				{Desc: "summary --no-color=false still prints none", args: []string{"summary", "--no-color=false", "../testdata/passing/"}, exit: 0},
				{Desc: "spec prints no color codes", args: []string{"spec", "../testdata/passing/"}, exit: 0},
				{Desc: "NO_COLOR is honoured as well", env: []string{"NO_COLOR=1"}, args: []string{"summary", "../testdata/passing/"}, exit: 0},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", row.env, row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stderr: %s", run.Stderr)
				gotest.NotContains(sub, run.Stdout, "\x1b[")
			}
		})
	})
}

func (s *CLIStreamsTestSuite) TestTwoOutputModesAreRefused(t *gotest.T) {
	t.When("two output modes are asked for at once", func(w *gotest.T) {
		w.It("refuses them, and an unknown --format, with exit 2", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "--spec with -json", args: []string{"--spec", "-json", "../testdata/passing/"}, exit: 2, stderrHas: "-json"},
				{Desc: "summary with -json", args: []string{"summary", "-json", "../testdata/passing/"}, exit: 2, stderrHas: "-json"},
				{Desc: "an unknown --format", args: []string{"summary", "--format", "xml", "../testdata/passing/"}, exit: 2, stderrHas: `unknown --format "xml"`},
				{Desc: "an unknown --format for spec", args: []string{"spec", "--format=yaml", "../testdata/passing/"}, exit: 2, stderrHas: `unknown --format "yaml"`},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, row.stderrHas)
			}
		})
	})
}
