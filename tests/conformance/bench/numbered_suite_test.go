package bench_test

import (
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchNumberedTestSuite: a benchmark whose name ends in a number keeps it
// when the run's lines carry no GOMAXPROCS suffix.
type BenchNumberedTestSuite struct{}

func (s *BenchNumberedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second, Parallel: true}
}

func (s *BenchNumberedTestSuite) TestASubBenchmarkNamedWithANumber(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "--json", "../testdata/benchnumbered/", "-cpu=1", "-count=2", "-benchtime=1x")
	gotest.NoError(t, err)
	var report struct {
		Baseline struct {
			GOMAXPROCS int `json:"gomaxprocs"`
			Results    []struct {
				Name    string            `json:"name"`
				Samples []json.RawMessage `json:"samples"`
			} `json:"results"`
		} `json:"baseline"`
	}

	t.When("gotest bench --json runs a sub-benchmark named size-1024 with -cpu=1 and -count=2", func(w *gotest.T) {
		w.It("the report names it size-1024 with two samples", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &report), "stdout: %s", run.Stdout)
			gotest.Len(it, report.Baseline.Results, 2, "stdout: %s", run.Stdout)
			gotest.Equal(it, "BenchmarkCopy/size-1024", report.Baseline.Results[1].Name)
			gotest.Len(it, report.Baseline.Results[1].Samples, 2, "stdout: %s", run.Stdout)
		})
		w.It("records no GOMAXPROCS of 1024", func(it *gotest.T) {
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &report), "stdout: %s", run.Stdout)
			gotest.NotEqual(it, 1024, report.Baseline.GOMAXPROCS)
		})
	})
}

func (s *BenchNumberedTestSuite) TestARunOverTwoProcessorCounts(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "--json", "../testdata/benchnumbered/", "-cpu=8,1", "-count=2", "-benchtime=1x")
	gotest.NoError(t, err)

	t.When("gotest bench --json runs sub-benchmarks named plain and size-1024 with -cpu=8,1 and -count=2", func(w *gotest.T) {
		w.It("the report keeps four samples of each, two at 8 processors and two at 1", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			var report struct {
				Baseline struct {
					Results []struct {
						Name    string `json:"name"`
						Samples []struct {
							Procs int `json:"procs"`
						} `json:"samples"`
					} `json:"results"`
				} `json:"baseline"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &report), "stdout: %s", run.Stdout)
			gotest.Len(it, report.Baseline.Results, 2, "stdout: %s", run.Stdout)
			for i, name := range []string{"BenchmarkCopy/plain", "BenchmarkCopy/size-1024"} {
				r := report.Baseline.Results[i]
				gotest.Equal(it, name, r.Name)
				var procs []int
				for _, sample := range r.Samples {
					procs = append(procs, sample.Procs)
				}
				gotest.Equal(it, []int{8, 8, 1, 1}, procs, "%s", name)
			}
		})
	})
}
