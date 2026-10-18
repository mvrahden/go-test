package cli_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRejectionsTestSuite: a suite the generator refuses is refused once,
// with every reason in one error.
type CLIRejectionsTestSuite struct{}

func (s *CLIRejectionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type rejectionRow struct {
	Desc     string
	pkg      string
	at       string // file:line of the first offending call
	hooks    string // the hooks the error names, in source order
	wrappers string
}

func (s *CLIRejectionsTestSuite) TestHooksCallingT(t *gotest.T) {
	t.When("several hooks call t.T() in a suite with fuzz or benchmark methods", func(w *gotest.T) {
		w.It("fails with exit 2 and one error on stderr that names every such hook in source order, located at the first", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []rejectionRow{
				{Desc: "fuzz methods", pkg: "../testdata/clihookcalls/", at: "hookcalls_suite_test.go:11:", hooks: "AfterEach and BeforeAll call t.T()", wrappers: "fuzz"},
				{Desc: "benchmark methods", pkg: "../testdata/clihookbench/", at: "hookbench_suite_test.go:11:", hooks: "AfterAll and BeforeEach call t.T()", wrappers: "benchmark"},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.pkg)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Equal(sub, 1, strings.Count(run.Stderr, "t.T(), which is nil"), "stderr: %s", run.Stderr)
				gotest.Contains(sub, run.Stderr, row.at)
				gotest.Contains(sub, run.Stderr, "has "+row.wrappers+" methods but "+row.hooks)
			}
		})
	})
}
