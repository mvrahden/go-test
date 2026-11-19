package bench_test

import (
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchZeroTestSuite: a sample that measured no time is a sample.
type BenchZeroTestSuite struct{}

func (s *BenchZeroTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second}
}

func (s *BenchZeroTestSuite) TestASampleThatMeasuredNoTime(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "--json", "../testdata/benchzero/", "-count=2", "-benchtime=1x")
	gotest.NoError(t, err)

	t.When("gotest bench --json runs a benchmark whose samples measured no time with -count=2", func(w *gotest.T) {
		w.It("the report keeps both samples at 0 ns/op", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			var report struct {
				Baseline struct {
					Results []struct {
						Name    string `json:"name"`
						Samples []struct {
							Iterations int     `json:"iterations"`
							NsPerOp    float64 `json:"nsPerOp"`
						} `json:"samples"`
					} `json:"results"`
				} `json:"baseline"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &report), "stdout: %s", run.Stdout)
			gotest.Len(it, report.Baseline.Results, 1, "stdout: %s", run.Stdout)
			r := report.Baseline.Results[0]
			gotest.Equal(it, "BenchmarkIdle", r.Name)
			gotest.Len(it, r.Samples, 2, "stdout: %s", run.Stdout)
			for _, sample := range r.Samples {
				gotest.Equal(it, 1, sample.Iterations)
				gotest.Zero(it, sample.NsPerOp)
			}
		})
	})
}
