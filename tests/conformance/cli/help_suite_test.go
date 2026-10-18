package cli_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIHelpTestSuite: what the help texts say about the flags they list.
type CLIHelpTestSuite struct{}

func (s *CLIHelpTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type helpRow struct {
	Desc string
	args []string
}

func (s *CLIHelpTestSuite) TestTheSetupTimeoutDefault(t *gotest.T) {
	t.When("a help text lists --setup-timeout", func(w *gotest.T) {
		w.It("gives its default as the fixtures' declared budgets, 2m until the first reports", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []helpRow{
				{Desc: "help", args: []string{"help"}},
				{Desc: "help watch", args: []string{"help", "watch"}},
				{Desc: "help spec", args: []string{"help", "spec"}},
				{Desc: "help summary", args: []string{"help", "summary"}},
				{Desc: "help bench", args: []string{"help", "bench"}},
				{Desc: "help config", args: []string{"help", "config"}},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				text := strings.Join(strings.Fields(run.Stdout), " ")
				gotest.Contains(sub, text, "setup-timeout")
				gotest.Contains(sub, text, "default: the fixtures' declared budgets, 2m until the first reports")
			}
		})
	})
}
