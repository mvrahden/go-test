package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldsTestSuite runs generated fixture packages under plain go test, where
// every suite shares one process and each top-level function holds the
// fixtures for itself.
type HoldsTestSuite struct{}

func (s *HoldsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

var plainPackages = []string{"./testdata/holds/", "./testdata/panics/"}

func (s *HoldsTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	binary, err := testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	for _, dir := range plainPackages {
		out, err := exec.Command(binary, "generate", dir).CombinedOutput() //nolint:gosec // G204: controlled binary with fixed args
		gotest.NoError(t, err, string(out))
	}
}

func (s *HoldsTestSuite) AfterAll(t *gotest.T) {
	for _, dir := range plainPackages {
		generated, _ := filepath.Glob(filepath.Join(dir, "gotest_p*suite_test.go"))
		for _, f := range generated {
			_ = os.Remove(f)
		}
	}
}

// ownEnv is this process's environment without the runner's sidecar files,
// which belong to this suite process, not to the ones it starts.
func ownEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, protocol.EnvStopFile+"=") || strings.HasPrefix(kv, protocol.EnvTeardownBudgetFile+"=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// goTest runs go test on dir and returns its exit code, its output and the
// fixture's setup and teardown lines, which it logs to the file logEnv names.
func goTest(t *gotest.T, dir, logEnv string, env []string, args ...string) (code int, out string, events []string) {
	log := filepath.Join(t.TempDir(), "events")
	cmd := exec.Command("go", append(append([]string{"test", "-count=1", "-v"}, args...), dir)...) //nolint:gosec // G204: fixed args
	cmd.Env = append(append(ownEnv(), logEnv+"="+log), env...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		exit := gotest.ErrorAs[*exec.ExitError](t, err)
		code = exit.ExitCode()
	}
	if b, err := os.ReadFile(log); err == nil {
		events = strings.Fields(string(b))
	}
	return code, string(raw), events
}

func holds(t *gotest.T, env []string, args ...string) (code int, out string, events []string) {
	return goTest(t, "./testdata/holds/", "HOLDS_LOG", env, args...)
}

// Every fixture-bound top-level function holds the fixture once: TestSeeded,
// its fuzz seed replay, TestPanicking and TestExt. Each teardown precedes the
// next setup, so no function sees what another one held.
func (s *HoldsTestSuite) TestEachTopLevelFunctionHoldsTheFixture(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc  string
		flags []string
		holds int
	}{
		{"a plain run", nil, 4},
		{"two shuffled rounds", []string{"-count=2", "-shuffle=on"}, 8},
	}) {
		code, out, events := holds(t, nil, tc.flags...)
		gotest.Equal(t, 0, code, out)
		gotest.Len(t, events, 2*tc.holds, "%q", events)
		for i, e := range events {
			gotest.Equal(t, []string{"setup", "teardown"}[i%2], e, "event %d of %q", i, events)
		}
	}
}

func (s *HoldsTestSuite) TestARunThatSelectsNoBoundSuiteSetsNothingUp(t *gotest.T) {
	for _, flags := range [][]string{{"-run", "TestUnboundTestSuite"}, {"-list", "."}} {
		code, out, events := holds(t, nil, flags...)
		gotest.Equal(t, 0, code, "go test %v\n%s", flags, out)
		gotest.Empty(t, events, "go test %v", flags)
	}
}

// The failure lands on the function that held the fixture.
func (s *HoldsTestSuite) TestATeardownFailureFailsTheHoldingTest(t *gotest.T) {
	code, out, _ := holds(t, []string{"HOLDS_FAIL_TEARDOWN=1"}, "-run", "TestPanickingTestSuite")
	gotest.Equal(t, 1, code, out)
	gotest.Regexp(t, `fixture teardown failed\n--- FAIL: TestPanickingTestSuite `, out)
}

func (s *HoldsTestSuite) TestAPanickingTestReleasesTheFixture(t *gotest.T) {
	code, out, events := holds(t, []string{"HOLDS_PANIC=1"}, "-run", "TestPanickingTestSuite")
	gotest.NotEqual(t, 0, code, "exit 0 after a panic\n%s", out)
	gotest.Contains(t, out, "panic: method panic")
	gotest.Equal(t, []string{"setup", "teardown"}, events)
}

// The testing package skips the parent's cleanups on a panicking
// sub-benchmark; the generated guard releases the fixture instead.
func (s *HoldsTestSuite) TestAPanickingBenchmarkReleasesTheFixture(t *gotest.T) {
	code, out, events := holds(t, []string{"HOLDS_BENCH_PANIC=1"}, "-run", "^$", "-bench", "BenchmarkPanickingTestSuite")
	gotest.NotEqual(t, 0, code, "exit 0 after a panic\n%s", out)
	gotest.Contains(t, out, "panic: benchmark panic")
	gotest.Contains(t, out, "BenchmarkPanickingTestSuite/BenchmarkPanics panicked; releasing fixtures")
	gotest.Regexp(t, `\.BenchmarkPanics\(`, out, "the traceback starts where the panic did")
	gotest.Equal(t, []string{"setup", "teardown"}, events)
}

// A panic releases the fixture wherever it happens below the function that
// holds it: in a subtest or cleanup gotest did not make, in a parallel
// subtest that runs after its parent's body returned, or in a lifecycle hook
// on the fuzz or benchmark path.
func (s *HoldsTestSuite) TestEveryPanicReleasesTheFixture(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc     string
		scenario string
		args     []string
		value    string
	}{
		{"a raw subtest", "raw", []string{"-run", "TestRawTestSuite"}, "raw subtest panic"},
		{"a parallel raw subtest", "parallel", []string{"-run", "TestParallelTestSuite"}, "parallel subtest panic"},
		{"a user cleanup", "cleanup", []string{"-run", "TestCleanupTestSuite"}, "cleanup panic"},
		{"BeforeAll on the fuzz path", "fuzzbeforeall", []string{"-run", "FuzzFuzzSetupTestSuite_FuzzInput"}, "fuzz BeforeAll panic"},
		{"AfterAll on the benchmark path", "benchafterall", []string{"-run", "^$", "-bench", "BenchmarkBenchTeardownTestSuite", "-benchtime=1x"}, "bench AfterAll panic"},
	}) {
		code, out, events := goTest(t, "./testdata/panics/", "PANICS_LOG", []string{"PANICS=" + tc.scenario}, tc.args...)
		gotest.NotEqual(t, 0, code, out)
		gotest.Contains(t, out, tc.value)
		gotest.Equal(t, []string{"setup", "teardown"}, events, out)
	}
}
