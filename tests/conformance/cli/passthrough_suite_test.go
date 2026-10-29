package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIPassthroughTestSuite: flags gotest does not recognise belong to the test
// binary, as under go test. After a bare "--" they travel with the package
// patterns beside them; after "-args" everything travels verbatim and the
// "-args" word itself is consumed.
type CLIPassthroughTestSuite struct{}

func (s *CLIPassthroughTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

func (s *CLIPassthroughTestSuite) TestUnknownFlagsReachTheBinary(t *gotest.T) {
	t.When("a flag only the test binary declares is passed", func(w *gotest.T) {
		w.It("reaches the test binary after -- or -args, and the binary fails its check without it", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "after -- it reaches the binary and the pattern stays a package", args: []string{"--", "-customflag=yes", "../testdata/customflag/"}, exit: 0},
				{Desc: "after -args it reaches the binary verbatim", args: []string{"../testdata/customflag/", "-args", "-customflag=yes"}, exit: 0},
				{Desc: "without it the binary's check fails", args: []string{"../testdata/customflag/"}, exit: 1, stderrHas: ""},
				{Desc: "spec puts its own -v ahead of the binary's words", args: []string{"spec", "../testdata/customflag/", "-args", "-customflag=yes"}, exit: 0},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.NotContains(sub, run.Stdout+run.Stderr, "flag provided but not defined: -args")
			}
		})
	})
}
