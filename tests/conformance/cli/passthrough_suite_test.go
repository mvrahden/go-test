package cli_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIPassthroughTestSuite: flags gotest does not recognise belong to the test
// binary, as under go test. After a bare "--" they travel with the package
// patterns beside them; after "-args" everything travels verbatim and the
// "-args" word itself is consumed.
type CLIPassthroughTestSuite struct{}

func (s *CLIPassthroughTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *CLIPassthroughTestSuite) TestUnknownFlagsReachTheBinary(t *gotest.T) {
	t.When("a flag only the test binary declares is passed", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []flagRow{
			{Desc: "after -- it reaches the binary and the pattern stays a package", args: []string{"--", "-customflag=yes", "../testdata/customflag/"}, exit: 0},
			{Desc: "after -args it reaches the binary verbatim", args: []string{"../testdata/customflag/", "-args", "-customflag=yes"}, exit: 0},
			{Desc: "without it the binary's check fails", args: []string{"../testdata/customflag/"}, exit: 1, stderrHas: ""},
			{Desc: "spec puts its own -v ahead of the binary's words", args: []string{"spec", "../testdata/customflag/", "-args", "-customflag=yes"}, exit: 0},
		}) {
			run, err := testkit.RunCLI(it.Context(), ".", row.args...)
			gotest.NoError(it, err)
			gotest.Equal(it, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.NotContains(it, run.Stdout+run.Stderr, "flag provided but not defined: -args")
		}
	})
}
