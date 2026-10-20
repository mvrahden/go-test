package cli_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIBuildRulesTestSuite: gotest builds a package under the same rules as
// go test, adding no flag of its own that would let a build pass go test
// refuses.
type CLIBuildRulesTestSuite struct{}

func (s *CLIBuildRulesTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline}
}

func (s *CLIBuildRulesTestSuite) TestLinknameIsCheckedAsGoTestChecksIt(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "../testdata/linkname/")
	gotest.NoError(t, err)
	t.When("a package references a runtime symbol through a linkname the linker refuses", func(w *gotest.T) {
		w.It("fails to build with exit 2, as under go test", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout+run.Stderr, "linkname")
		})
	})
}
