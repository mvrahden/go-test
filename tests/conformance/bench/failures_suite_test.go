package bench_test

import (
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchFailuresTestSuite: the --json report carries what a failed benchmark
// printed, not only the samples of those that passed.
type BenchFailuresTestSuite struct{}

func (s *BenchFailuresTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second}
}

type benchFailure struct {
	Package string `json:"package"`
	Suite   string `json:"suite"`
	Name    string `json:"name"`
	Output  string `json:"output"`
}

func (s *BenchFailuresTestSuite) TestAPanickingBenchmark(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "--json", "../testdata/benchpanic/", "-benchtime=1x")
	gotest.NoError(t, err)

	t.When("a benchmark panics in a --json bench run", func(w *gotest.T) {
		w.It("the report's failures name the benchmark with the panic's message", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			var report struct {
				Failures []benchFailure `json:"failures"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &report), "stdout: %s", run.Stdout)
			gotest.Len(it, report.Failures, 1, "stdout: %s", run.Stdout)
			f := report.Failures[0]
			gotest.Regexp(it, `/benchpanic$`, f.Package)
			gotest.Equal(it, "PanickingBenchTestSuite", f.Suite)
			gotest.Equal(it, "BenchmarkPanics", f.Name)
			gotest.Contains(it, f.Output, "the benchmark panics on purpose")
		})
	})
}
