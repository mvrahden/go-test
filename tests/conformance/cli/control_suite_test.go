package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ControlFlagTestSuite: --control takes stdin on every command that stops on
// a SIGINT, and nothing else. Every run here has its standard input at its
// end, which stops a command still running.
type ControlFlagTestSuite struct{}

func (s *ControlFlagTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *ControlFlagTestSuite) TestAValueOtherThanStdin(t *gotest.T) {
	t.When("--control is given a value other than stdin", func(w *gotest.T) {
		w.It("refuses it with exit 2, naming --control and stdin", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "a run", args: []string{"--control=pipe", "../testdata/passing/"}},
				{Desc: "an empty value", args: []string{"--control=", "../testdata/passing/"}},
				{Desc: "prepare", args: []string{"prepare", "--control=fd3", "../testdata/passing/"}},
				{Desc: "fuzz triage", args: []string{"fuzz", "triage", "--control", "tty", "../testdata/fuzzing/"}},
				{Desc: "spec with --input", args: []string{"spec", "--control=file", "--input=events.json"}},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Regexp(sub, `invalid --control value ".*": its one value is stdin`, run.Stderr)
			}
		})
	})
}

// controlRow is one command given --control=stdin; anyExit is for a fuzz
// session, whose exit depends on how far its start got before the stop.
type controlRow struct {
	Desc      string
	args      []string
	exit      int
	anyExit   bool
	stdoutHas string
	stderrHas string
}

func (s *ControlFlagTestSuite) TestEveryCommandThatStopsOnASIGINT(t *gotest.T) {
	t.When("--control=stdin is given to each command that stops on a SIGINT, its standard input at its end", func(w *gotest.T) {
		w.It("takes the flag and ends, stopped by the input's end where it was still running", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []controlRow{
				{Desc: "a run", args: []string{"--control=stdin", "../testdata/passing/"}, exit: 130},
				{Desc: "spec", args: []string{"spec", "--control=stdin", "../testdata/passing/"}, exit: 130},
				{Desc: "summary", args: []string{"summary", "--control=stdin", "../testdata/passing/"}, exit: 130},
				{Desc: "watch", args: []string{"watch", "--control=stdin", "../testdata/passing/"}, exit: 130},
				{Desc: "bench", args: []string{"bench", "--control=stdin", "../testdata/benching/"}, exit: 130},
				{Desc: "fuzz", args: []string{"fuzz", "--control=stdin", "../testdata/fuzzing/"}, anyExit: true, stderrHas: "fuzzed 1 target in"},
				{Desc: "fuzz triage", args: []string{"fuzz", "triage", "--control=stdin", "../testdata/fuzzing/"}, exit: 0, stdoutHas: "no crashers found"},
				{Desc: "fuzz promote", args: []string{"fuzz", "promote", "--control", "stdin", "../testdata/fuzzing/"}, exit: 0},
				{Desc: "prepare, which waits for a stop", args: []string{"prepare", "--control=stdin", "../testdata/passing/"}, exit: 0, stdoutHas: `"overlayFile"`},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				if !row.anyExit {
					gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				}
				gotest.NotContains(sub, run.Stderr, "--control")
				if row.stdoutHas != "" {
					gotest.Contains(sub, run.Stdout, row.stdoutHas)
				}
				if row.stderrHas != "" {
					gotest.Contains(sub, run.Stderr, row.stderrHas)
				}
			}
		})
	})
}

func (s *ControlFlagTestSuite) TestBesideInputFromStdin(t *gotest.T) {
	t.When("--control=stdin is given beside --input=-", func(w *gotest.T) {
		w.It("refuses it with exit 2: both read standard input", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "spec", args: []string{"spec", "--control=stdin", "--input=-"}},
				{Desc: "summary", args: []string{"summary", "--input=-", "--control=stdin"}},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, "--control=stdin and --input=- both read standard input")
			}
		})
	})
}
