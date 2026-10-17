package cli_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIVersionTestSuite: the one invocation every other check assumes works.
type CLIVersionTestSuite struct{}

func (s *CLIVersionTestSuite) TestVersion(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "version")
	gotest.NoError(t, err)
	t.When("the version subcommand runs", func(w *gotest.T) {
		w.It("exits 0 and prints the version on stdout", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit)
			gotest.Regexp(it, "gotest", run.Stdout)
			gotest.Empty(it, run.Stderr)
		})
	})
}
