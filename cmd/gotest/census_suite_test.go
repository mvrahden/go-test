package main_test

import (
	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CensusTestSuite runs the pipeline's census end to end through the commands
// over tiny fixture packages, where a misjudged census would turn green into 2.
type CensusTestSuite struct{}

// SuiteConfig: every behavior compiles and runs a package through a
// command, which the 30-second default does not cover on a slow machine.
func (s *CensusTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *CensusTestSuite) TestRunsAreCensused(t *gotest.T) {
	t.When("a suite skips itself in BeforeAll", func(w *gotest.T) {
		w.It("summary stays green: the skip covers the suite's methods", func(it *gotest.T) {
			gotest.Equal(it, 0, main.ExportRunSummary(main.Invocation{Args: []string{"./testdata/census/skipped/"}}))
		})
	})

	t.When("the run is filtered to nothing", func(w *gotest.T) {
		w.It("spec stands down instead of reporting every method missing", func(it *gotest.T) {
			gotest.Equal(it, 0, main.ExportRunSpec(main.Invocation{Args: []string{"./testdata/census/plain/", "-run", "TestNothingMatches"}}))
		})
	})

	t.When("a package declares a fuzz target", func(w *gotest.T) {
		w.It("spec replays its seeds and the census counts the wrapper", func(it *gotest.T) {
			gotest.Equal(it, 0, main.ExportRunSpec(main.Invocation{Args: []string{"./testdata/census/fuzzplain/"}}))
		})
	})

	t.When("a plain package runs in -json mode", func(w *gotest.T) {
		w.It("is censused as the stream is written and stays green", func(it *gotest.T) {
			gotest.Equal(it, 0, main.Run(main.ExecConfig{PackagePatterns: []string{"./testdata/census/plain/"}, JSON: true}))
		})
	})
}

func (s *CensusTestSuite) TestBenchRunsAreCensused(t *gotest.T) {
	t.When("a bench run renders the spec", func(w *gotest.T) {
		w.It("stays green when every declared benchmark produced a result", func(it *gotest.T) {
			gotest.Equal(it, 0, main.ExportRunBench(main.Invocation{Args: []string{"--spec", "--no-color", "./testdata/census/benchplain/", "-benchtime=1x"}}))
		})
	})

	t.When("the caller selects benchmarks with -bench", func(w *gotest.T) {
		w.It("stands down instead of reporting the unselected ones", func(it *gotest.T) {
			gotest.Equal(it, 0, main.ExportRunBench(main.Invocation{Args: []string{"--spec", "--no-color", "./testdata/census/benchplain/", "-benchtime=1x", "-bench=^BenchmarkBenchPlainTestSuite$/^BenchmarkA$"}}))
		})
	})
}
