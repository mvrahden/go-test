package cli_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIGoTestFlagsTestSuite: the go test flags gotest takes over keep the
// meaning go test gives them, or are refused with a reason.
type CLIGoTestFlagsTestSuite struct{}

func (s *CLIGoTestFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *CLIGoTestFlagsTestSuite) TestOwnedGoTestFlags(t *gotest.T) {
	t.When("a go test flag gotest implements itself is given", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []flagRow{
			{Desc: "-run naming a fuzz wrapper selects its suite and replays the seeds", args: []string{"-v", "-run", "FuzzFuzzyTestSuite_FuzzUpperIsIdempotent", "../testdata/fuzzing/"}, exit: 0, stdoutHas: "--- PASS: FuzzFuzzyTestSuite_FuzzUpperIsIdempotent"},
			{Desc: "-exec wraps every suite binary", args: []string{"-exec", "env", "../testdata/passing/"}, exit: 0, stdoutHas: "ok"},
			{Desc: "-p caps the processes and the run still completes", args: []string{"-p", "1", "../testdata/passing/", "../testdata/covered/"}, exit: 0},
			{Desc: "-o is refused with a reason", args: []string{"-o", "out.test", "../testdata/passing/"}, exit: 2, stderrHas: "-o"},
			{Desc: "-buildvcs=false reaches the build", args: []string{"-buildvcs=false", "../testdata/passing/"}, exit: 0, stdoutHas: "ok"},
			{Desc: "-pgo=off reaches the build", args: []string{"-pgo=off", "../testdata/passing/"}, exit: 0, stdoutHas: "ok"},
		}) {
			run, err := testkit.RunCLI(it.Context(), ".", row.args...)
			gotest.NoError(it, err)
			gotest.Equal(it, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			if row.stdoutHas != "" {
				gotest.Contains(it, run.Stdout, row.stdoutHas)
			}
			if row.stderrHas != "" {
				gotest.Contains(it, run.Stderr, row.stderrHas)
			}
		}
	})
}
