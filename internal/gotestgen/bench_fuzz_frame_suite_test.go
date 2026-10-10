package gotestgen_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Each wrapper kind runs alone: a suite without Test methods still gets a Test
// wrapper, which would print the same markers.
var (
	benchOnly = []string{"-test.run=^$", "-test.bench=.", "-test.benchtime=1x"}
	fuzzOnly  = []string{"-test.run=^Fuzz"}
)

// BenchFuzzFrameTestSuite runs generated Benchmark and Fuzz wrappers and checks
// they open their suite on the same terms as a Test wrapper.
type BenchFuzzFrameTestSuite struct{}

func (s *BenchFuzzFrameTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	cfg.Timeout = 3 * time.Minute
	return cfg
}

func (s *BenchFuzzFrameTestSuite) TestBenchmarkLifecycleContexts(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_BenchFuzzFrame", childTimeout, benchOnly...)

	t.It("passes", func(it *gotest.T) {
		assertNoDeadlock(it, run)
		gotest.True(it, run.passed, run.output)
		gotest.NotContains(it, run.output, "MARK:fuzz", run.output)
	})

	t.It("bounds BeforeAll and AfterAll by the declared SetupTimeout", func(it *gotest.T) {
		gotest.Contains(it, run.output, "MARK:bench beforeall err=<nil> deadline 2m0s", run.output)
		gotest.Contains(it, run.output, "MARK:bench afterall err=<nil> deadline 2m0s", run.output)
	})
}

func (s *BenchFuzzFrameTestSuite) TestFuzzLifecycleContexts(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, "TestLifecycle_BenchFuzzFrame", childTimeout, fuzzOnly...)

	t.It("passes", func(it *gotest.T) {
		assertNoDeadlock(it, run)
		gotest.True(it, run.passed, run.output)
		gotest.NotContains(it, run.output, "MARK:bench", run.output)
	})

	t.It("bounds BeforeAll and AfterAll by the default SetupTimeout", func(it *gotest.T) {
		gotest.Contains(it, run.output, "MARK:fuzz beforeall err=<nil> deadline 30s", run.output)
		gotest.Contains(it, run.output, "MARK:fuzz afterall err=<nil> deadline 30s", run.output)
	})
}

func (s *BenchFuzzFrameTestSuite) TestSetupOverrun(t *gotest.T) {
	for _, c := range []struct {
		kind, wrapper string
		args          []string
	}{
		{"benchmark", "BenchmarkBenchOverrunTestSuite", benchOnly},
		{"fuzz", "FuzzFuzzOverrunTestSuite_FuzzNoop", fuzzOnly},
	} {
		t.It("fails the "+c.kind+" wrapper and names it", func(it *gotest.T) {
			run := runGeneratedSuiteArgs(it, "TestLifecycle_BenchFuzzSetupOverrun", childTimeout, c.args...)
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, run.output)
			gotest.Contains(it, run.output, "gotest: "+c.wrapper+" BeforeAll exceeded its configured SetupTimeout", run.output)
		})
	}
}

func (s *BenchFuzzFrameTestSuite) TestBenchmarkAfterEach(t *gotest.T) {
	for _, c := range []struct{ how, bench string }{
		{"fails", "BenchmarkFails"},
		{"panics", "BenchmarkPanics"},
	} {
		t.It("runs when the benchmark "+c.how, func(it *gotest.T) {
			run := runGeneratedSuiteArgs(it, "TestLifecycle_BenchAfterEach", childTimeout,
				"-test.run=^$", "-test.bench=/"+c.bench+"$", "-test.benchtime=1x")
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, run.output)
			gotest.Contains(it, run.output, "benchmark "+c.how+" on purpose", run.output)
			gotest.Contains(it, run.output, "MARK:aftereach ran", run.output)
		})
	}
}
