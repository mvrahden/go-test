package lifecycle_test

import (
	"encoding/json"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchPanicTestSuite: a benchmark that panics is a failed benchmark, not a
// lost one; what comes after it still runs and tears down.
type BenchPanicTestSuite struct{}

func (s *BenchPanicTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

func (s *BenchPanicTestSuite) TestAPanickingBenchmark(t *gotest.T) {
	dir := t.TempDir()
	// The report names what measured; the plain run shows the panic.
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GOTEST_CANARY_DIR=" + dir}, "bench", "--json", "../testdata/benchpanic/", "-benchtime=1x")
	gotest.NoError(t, err)
	markers, err := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	gotest.NoError(t, err)
	plain, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GOTEST_CANARY_DIR=" + t.TempDir()}, "bench", "../testdata/benchpanic/", "-benchtime=1x")
	gotest.NoError(t, err)
	var report struct {
		Baseline struct {
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
		} `json:"baseline"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &report), "stdout is not a report:\n%s", run.Stdout)
	var names []string
	for _, r := range report.Baseline.Results {
		names = append(names, r.Name)
	}
	t.When("a benchmark panics", func(w *gotest.T) {
		w.It("fails the run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, 1, plain.Exit, "stdout: %s\nstderr: %s", plain.Stdout, plain.Stderr)
		})
		w.It("prints the panic with its stack on stderr", func(it *gotest.T) {
			gotest.Contains(it, plain.Stderr, "panic: the benchmark panics on purpose")
			gotest.Contains(it, plain.Stderr, "goroutine ")
		})
		w.It("reports no result for it and a result for the benchmark after it", func(it *gotest.T) {
			gotest.Equal(it, []string{"BenchmarkStillRuns"}, names)
			gotest.Contains(it, markers, "still runs")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, testkit.MarkerOrderOf(markers, "panics", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", markers)
		})
	})
}
