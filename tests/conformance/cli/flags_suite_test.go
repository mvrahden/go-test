package cli_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIFlagsTestSuite: every gotest flag is read the same way by every command.
// A boolean takes --x, --x=true and --x=false, the last occurrence wins, a
// value flag without a value and an unknown flag are usage errors, and the
// commands that take packages validate their flags before doing anything.
type CLIFlagsTestSuite struct{}

func (s *CLIFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type flagRow struct {
	Desc      string
	args      []string
	exit      int
	stdoutHas string
	stdoutNot string
	stderrHas string
}

func (s *CLIFlagsTestSuite) TestBooleanFlagSpellings(t *gotest.T) {
	t.When("a boolean flag is spelled in each accepted form", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []flagRow{
			{Desc: "--ci guards a focused run", args: []string{"--ci", "../testdata/focused/"}, exit: 1, stderrHas: "F_"},
			{Desc: "--ci=true guards a focused run", args: []string{"--ci=true", "../testdata/focused/"}, exit: 1, stderrHas: "F_"},
			{Desc: "--ci=false lets a focused run pass", args: []string{"--ci=false", "../testdata/focused/"}, exit: 0},
			{Desc: "the last occurrence wins", args: []string{"--ci", "--ci=false", "../testdata/focused/"}, exit: 0},
			{Desc: "a non-boolean value is a usage error", args: []string{"--ci=maybe", "../testdata/focused/"}, exit: 2, stderrHas: `invalid boolean value "maybe" for --ci`},
			{Desc: "summary --no-color=true prints no color codes", args: []string{"summary", "--no-color=true", "../testdata/passing/"}, exit: 0, stdoutNot: "\x1b["},
		}) {
			run, err := testkit.RunCLI(it.Context(), ".", row.args...)
			gotest.NoError(it, err)
			gotest.Equal(it, row.exit, run.Exit, "stderr: %s", run.Stderr)
			if row.stderrHas != "" {
				gotest.Contains(it, run.Stderr, row.stderrHas)
			}
			if row.stdoutNot != "" {
				gotest.NotContains(it, run.Stdout, row.stdoutNot)
			}
		}
	})
}

func (s *CLIFlagsTestSuite) TestValueFlagsAndUnknownFlags(t *gotest.T) {
	t.When("a value flag or an unknown flag is given to a command", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []flagRow{
			{Desc: "a value flag at the end without a value is a usage error", args: []string{"../testdata/passing/", "--min"}, exit: 2, stderrHas: "needs a value"},
			{Desc: "an unknown flag is a usage error for a run", args: []string{"--bogus", "../testdata/passing/"}, exit: 2, stderrHas: "--bogus"},
			{Desc: "an unknown flag is a usage error for clean", args: []string{"clean", "--bogus", "../testdata/passing/"}, exit: 2, stderrHas: "--bogus"},
			{Desc: "an unknown flag is a usage error for generate", args: []string{"generate", "--bogus", "../testdata/passing/"}, exit: 2, stderrHas: "--bogus"},
			{Desc: "an unknown flag is a usage error for discover", args: []string{"discover", "--bogus", "../testdata/passing/"}, exit: 2, stderrHas: "--bogus"},
			{Desc: "an unknown flag is a usage error for prepare", args: []string{"prepare", "--bogus", "../testdata/passing/"}, exit: 2, stderrHas: "--bogus"},
			{Desc: "a flag another command owns is refused", args: []string{"clean", "--debug", "../testdata/passing/"}, exit: 2, stderrHas: "not valid for this subcommand"},
		}) {
			run, err := testkit.RunCLI(it.Context(), ".", row.args...)
			gotest.NoError(it, err)
			gotest.Equal(it, row.exit, run.Exit, "stdout: %s stderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, row.stderrHas)
		}
	})
}
