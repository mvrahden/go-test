package gotestrunner_test

import (
	"runtime"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OwnedGoTestFlagsTestSuite: -run, -p and -exec keep go test's meaning when
// gotest dispatches the suites itself.
type OwnedGoTestFlagsTestSuite struct{}

func (s *OwnedGoTestFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *OwnedGoTestFlagsTestSuite) TestRunFilterSelectsAFuzzWrapper(t *gotest.T) {
	compiled := []gotestrunner.CompileResult{{Package: "example.com/p", BinaryPath: "/bin/p.test"}}
	suites := map[string][]string{"example.com/p": {"FuzzyTestSuite", "OtherTestSuite"}}
	fuzz := map[string]map[string][]string{"example.com/p": {"FuzzyTestSuite": {"FuzzFuzzyTestSuite_FuzzUpper"}}}
	t.When("-run names a generated fuzz wrapper", func(w *gotest.T) {
		targets := gotestrunner.BuildSuiteTargets(compiled, suites, map[string]string{"example.com/p": "/src/p"}, fuzz, nil, nil, "FuzzFuzzyTestSuite_FuzzUpper")
		w.It("selects the suite that owns it, and only that suite", func(it *gotest.T) {
			gotest.Len(it, targets, 1)
			gotest.Equal(it, "TestFuzzyTestSuite", targets[0].SuiteName)
		})
		w.It("runs the binary with the user's filter", func(it *gotest.T) {
			gotest.Equal(it, "FuzzFuzzyTestSuite_FuzzUpper", targets[0].RunFilter)
		})
		bulk, _ := gotestrunner.ExportPlanSuitePhases(suites, nil, fuzz, "FuzzFuzzyTestSuite_FuzzUpper")
		w.It("plans the suite's fixtures for the run", func(it *gotest.T) {
			gotest.Equal(it, []string{"TestFuzzyTestSuite"}, bulk["example.com/p"])
		})
	})
}

func (s *OwnedGoTestFlagsTestSuite) TestProcessCapAndExecPrefix(t *gotest.T) {
	t.When("-p is among the build flags", func(w *gotest.T) {
		w.It("is read as the process cap", func(it *gotest.T) {
			gotest.Equal(it, 2, gotestrunner.ExtractProcessCap([]string{"-race", "-p", "2"}))
			gotest.Equal(it, 3, gotestrunner.ExtractProcessCap([]string{"-p=3"}))
			gotest.Zero(it, gotestrunner.ExtractProcessCap([]string{"-race"}))
		})
		w.It("caps the dispatched processes", func(it *gotest.T) {
			runFlags := []string{"-v"}
			gotest.Equal(it, 1, gotestrunner.ExportComputeDispatchConcurrency(&runFlags, 0, 64, 1, false))
		})
	})
	t.When("-exec is among the build flags", func(w *gotest.T) {
		w.It("is lifted out as the command the binaries run under", func(it *gotest.T) {
			prefix, rest := gotestrunner.ExtractExecPrefix([]string{"-race", "-exec", "qemu-aarch64 -L /sysroot", "-tags=x"})
			gotest.Equal(it, []string{"qemu-aarch64", "-L", "/sysroot"}, prefix)
			gotest.Equal(it, []string{"-race", "-tags=x"}, rest)
		})
		w.It("leads the test2json command line ahead of the binary", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{BinaryPath: "/bin/p.test", Exec: []string{"env", "X=1"}}
			target.Package = "example.com/p"
			argv := gotestrunner.ExportTest2JSONArgv("/go/pkg/tool/test2json", target, []string{"-test.v=test2json"})
			gotest.Equal(it, []string{"/go/pkg/tool/test2json", "-p", "example.com/p", "-t", "env", "X=1", "/bin/p.test", "-test.v=test2json"}, argv)
		})
	})
}

func (s *OwnedGoTestFlagsTestSuite) TestUserParallelKeepsTheBudget(t *gotest.T) {
	procs := runtime.GOMAXPROCS(0)
	t.When("-parallel is pinned by the user", func(w *gotest.T) {
		w.It("divides the default budget by it instead of doubling the processes", func(it *gotest.T) {
			runFlags := []string{"-parallel=4"}
			got := gotestrunner.ExportComputeDispatchConcurrency(&runFlags, 0, 64, 0, false)
			gotest.Equal(it, min(64, procs, max(1, 2*procs/4)), got)
		})
		w.It("divides an explicit budget by it", func(it *gotest.T) {
			runFlags := []string{"-parallel=4"}
			got := gotestrunner.ExportComputeDispatchConcurrency(&runFlags, 12, 64, 0, false)
			gotest.Equal(it, min(64, procs, 3), got)
		})
	})
}
