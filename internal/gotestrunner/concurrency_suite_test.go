package gotestrunner_test

import (
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ConcurrencyTestSuite covers the concurrency budget: process and compile widths, their
// sanitizer halving, exclusive dispatch and the slow-build notice.
type ConcurrencyTestSuite struct{}

func (s *ConcurrencyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ConcurrencyTestSuite) TestExclusiveDispatch(t *gotest.T) {
	t.When("BuildSuiteTargets sees a suite marked exclusive", func(w *gotest.T) {
		compiled := []gotestrunner.CompileResult{{Package: "example.com/pkg", BinaryPath: "/tmp/pkg.test"}}
		suitesByPkg := map[string][]string{"example.com/pkg": {"TimingTestSuite", "PlainTestSuite"}}
		exclusiveByPkg := map[string]map[string]bool{"example.com/pkg": {"TimingTestSuite": true}}
		targets := gotestrunner.BuildSuiteTargets(compiled, suitesByPkg, map[string]string{"example.com/pkg": "/src"}, nil, exclusiveByPkg, nil, "")

		w.It("carries the flag on exactly that suite's target", func(it *gotest.T) {
			byName := map[string]bool{}
			for i := range targets {
				byName[targets[i].SuiteName] = targets[i].Exclusive
			}
			gotest.True(it, byName["TestTimingTestSuite"])
			gotest.False(it, byName["TestPlainTestSuite"])
		})
	})

	t.When("ordering exclusive targets for serial dispatch", func(w *gotest.T) {
		w.It("sorts deterministically by package then suite name", func(it *gotest.T) {
			targets := []gotestrunner.SuiteTarget{
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "b", SuiteName: "TestZ"}},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a", SuiteName: "TestB"}},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a", SuiteName: "TestA"}},
			}
			idx := []int{0, 1, 2}
			gotestrunner.ExportSortTargetIndices(targets, idx)
			gotest.Equal(it, []int{2, 1, 0}, idx)
		})
	})
}

func (s *ConcurrencyTestSuite) TestSanitizerAwareDispatch(t *gotest.T) {
	procs := runtime.GOMAXPROCS(0)

	t.When("an instrumentation build flag is active with default parallelism", func(w *gotest.T) {
		w.It("halves the process cap so instrumented suites keep scheduling headroom", func(it *gotest.T) {
			cfg := gotestrunner.PipelineConfig{}
			runFlags := []string{}
			got := gotestrunner.ResolveBenchParallelismForTest(cfg, &runFlags, 64, true)
			gotest.Equal(it, max(1, procs/2), got)
		})

		w.It("keeps an explicit --parallel budget untouched — the user's number wins", func(it *gotest.T) {
			cfg := gotestrunner.PipelineConfig{Parallel: 10}
			runFlags := []string{}
			withRace := gotestrunner.ResolveBenchParallelismForTest(cfg, &runFlags, 64, true)
			runFlags = []string{}
			without := gotestrunner.ResolveBenchParallelismForTest(cfg, &runFlags, 64, false)
			gotest.Equal(it, without, withRace)
		})
	})

	t.When("detecting instrumentation from build flags", func(w *gotest.T) {
		w.It("recognizes -race, -msan and -asan and nothing else", func(it *gotest.T) {
			gotest.True(it, gotestrunner.SanitizerActive([]string{"-tags=integration", "-race"}))
			gotest.True(it, gotestrunner.SanitizerActive([]string{"-msan"}))
			gotest.True(it, gotestrunner.SanitizerActive([]string{"-asan"}))
			gotest.False(it, gotestrunner.SanitizerActive([]string{"-tags=integration", "-cover"}))
			gotest.False(it, gotestrunner.SanitizerActive(nil))
		})
	})
}

func (s *ConcurrencyTestSuite) TestComputeConcurrency(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name       string
		budget     int
		numSuites  int
		gomaxprocs int
		wantInter  int
		wantIntra  int
	}{
		// Default budget (2×GOMAXPROCS), 4 cores
		{"1 suite 4 cores", 8, 1, 4, 1, 8},
		{"2 suites 4 cores", 8, 2, 4, 2, 4},
		{"4 suites 4 cores", 8, 4, 4, 4, 2},
		{"20 suites 4 cores", 8, 20, 4, 4, 2},

		// Default budget (2×GOMAXPROCS), 8 cores
		{"2 suites 8 cores", 16, 2, 8, 2, 8},
		{"8 suites 8 cores", 16, 8, 8, 8, 2},
		{"30 suites 8 cores", 16, 30, 8, 8, 2},

		// Small budget (< GOMAXPROCS)
		{"budget 4 on 8 cores", 4, 20, 8, 4, 1},
		{"budget 2 on 8 cores", 2, 20, 8, 2, 1},

		// Large budget
		{"budget 32 on 4 cores", 32, 20, 4, 4, 8},

		// Edge: fewer suites than cores
		{"budget 16 1 suite", 16, 1, 8, 1, 16},

		// Edge: zero/negative budget uses default
		{"zero budget", 0, 10, 4, 4, 2},

		// Edge: zero suites
		{"zero suites", 8, 0, 4, 1, 8},
	}) {
		inter, intra := gotestrunner.ComputeConcurrency(tc.budget, tc.numSuites, tc.gomaxprocs)
		gotest.Equal(sub, tc.wantInter, inter, "inter")
		gotest.Equal(sub, tc.wantIntra, intra, "intra")
	}
}

func (s *ConcurrencyTestSuite) TestCompileConcurrency(t *gotest.T) {
	numCPU := runtime.NumCPU()

	for sub, tc := range gotest.Each(t, []struct {
		Name            string
		compileParallel int
		buildFlags      []string
		expect          int
	}{
		{"default without sanitizers", 0, []string{"-v", "-count=1"}, numCPU},
		{"race halves", 0, []string{"-race"}, max(1, numCPU/2)},
		{"msan halves", 0, []string{"-msan"}, max(1, numCPU/2)},
		{"asan halves", 0, []string{"-asan"}, max(1, numCPU/2)},
		{"race among other flags", 0, []string{"-v", "-race", "-count=1"}, max(1, numCPU/2)},
		{"explicit override honored", 3, []string{"-race"}, 3},
		{"explicit override without sanitizers", 6, []string{"-v"}, 6},
		{"no flags", 0, nil, numCPU},
	}) {
		got := gotestrunner.ExportCompileConcurrency(tc.compileParallel, tc.buildFlags)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *ConcurrencyTestSuite) TestResolveBenchParallelism(t *gotest.T) {
	t.When("resolving dispatch concurrency", func(w *gotest.T) {
		w.It("forces serial dispatch (1) in bench mode regardless of budget", func(it *gotest.T) {
			cfg := gotestrunner.PipelineConfig{Bench: true, Parallel: 8}
			runFlags := []string{}
			got := gotestrunner.ResolveBenchParallelismForTest(cfg, &runFlags, 5, false)
			gotest.Equal(it, 1, got)
		})

		w.It("does not inject -parallel into run flags in bench mode", func(it *gotest.T) {
			cfg := gotestrunner.PipelineConfig{Bench: true}
			runFlags := []string{"-v"}
			gotestrunner.ResolveBenchParallelismForTest(cfg, &runFlags, 5, false)
			gotest.Equal(it, []string{"-v"}, runFlags)
		})

		w.It("falls back to computeDispatchConcurrency (with intra-injection) outside bench mode", func(it *gotest.T) {
			cfg := gotestrunner.PipelineConfig{Bench: false}
			runFlags := []string{}
			got := gotestrunner.ResolveBenchParallelismForTest(cfg, &runFlags, 4, false)
			gotest.Greater(it, got, 0)
			found := false
			for _, f := range runFlags {
				if strings.HasPrefix(f, "-parallel") {
					found = true
				}
			}
			gotest.True(it, found, "expected -parallel to be injected outside bench mode, got %v", runFlags)
		})
	})
}

func (s *ConcurrencyTestSuite) TestLogSlowBuild(t *gotest.T) {
	t.When("a build outlives the threshold", func(w *gotest.T) {
		w.It("logs the breach while running and the effective duration after", func(it *gotest.T) {
			// A plain strings.Builder is the point: logSlowBuild owns the
			// synchronization between its timer goroutine and done(), so a
			// non-concurrent-safe writer must be race-free under -race.
			var buf strings.Builder
			done := gotestrunner.ExportLogSlowBuild(&buf, "test binary for example.com/pkg", 10*time.Millisecond)
			time.Sleep(40 * time.Millisecond)
			done()
			gotest.Contains(it, buf.String(), "has been building for 10ms and is still running")
			gotest.Contains(it, buf.String(), "finished building after")
		})
	})

	t.When("a build finishes under the threshold", func(w *gotest.T) {
		w.It("stays silent — fast builds are the expected case", func(it *gotest.T) {
			var buf strings.Builder
			done := gotestrunner.ExportLogSlowBuild(&buf, "x", time.Minute)
			done()
			gotest.Zero(it, buf.String())
		})
	})
}
