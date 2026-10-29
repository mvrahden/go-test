package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldsTestSuite runs generated fixture packages under plain go test, where
// every suite shares one process and each top-level function holds the
// fixtures for itself.
type HoldsTestSuite struct{ root string }

func (s *HoldsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

var plainPackages = []string{"./testdata/holds/", "./testdata/panics/"}

func (s *HoldsTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	binary, err := testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	gotest.NoError(t, testkit.PrivateCache(t.TempDir()))
	s.root = stageTestdata(t, root)
	for _, dir := range plainPackages {
		cmd := exec.Command(binary, "generate", dir) //nolint:gosec // G204: controlled binary with fixed args
		cmd.Dir = s.root
		out, err := cmd.CombinedOutput()
		gotest.NoError(t, err, string(out))
	}
}

// AfterAll has nothing to release: the staged module is a temp dir.
func (s *HoldsTestSuite) AfterAll(t *gotest.T) {}

// goTest runs go test on dir, relative to the staged module root, and returns its exit code, its output and the
// fixture's setup and teardown lines, which it logs to the file logEnv names.
func goTest(t *gotest.T, root, dir, logEnv string, env []string, args ...string) (code int, out string, events []string) {
	log := filepath.Join(t.TempDir(), "events")
	cmd := exec.Command("go", append(append([]string{"test", "-count=1", "-v"}, args...), dir)...) //nolint:gosec // G204: fixed args
	cmd.Dir = root
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

func (s *HoldsTestSuite) holds(t *gotest.T, env []string, args ...string) (code int, out string, events []string) {
	return goTest(t, s.root, "./testdata/holds/", "HOLDS_LOG", env, args...)
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
		code, out, events := s.holds(t, nil, tc.flags...)
		gotest.Equal(t, 0, code, out)
		gotest.Len(t, events, 2*tc.holds, "%q", events)
		for i, e := range events {
			gotest.Equal(t, []string{"setup", "teardown"}[i%2], e, "event %d of %q", i, events)
		}
	}
}

func (s *HoldsTestSuite) TestARunThatSelectsNoBoundSuiteSetsNothingUp(t *gotest.T) {
	for _, flags := range [][]string{{"-run", "TestUnboundTestSuite"}, {"-list", "."}} {
		code, out, events := s.holds(t, nil, flags...)
		gotest.Equal(t, 0, code, "go test %v\n%s", flags, out)
		gotest.Empty(t, events, "go test %v", flags)
	}
}

// The failure lands on the function that held the fixture.
func (s *HoldsTestSuite) TestATeardownFailureFailsTheHoldingTest(t *gotest.T) {
	code, out, _ := s.holds(t, []string{"HOLDS_FAIL_TEARDOWN=1"}, "-run", "TestPanickingTestSuite")
	gotest.Equal(t, 1, code, out)
	gotest.Regexp(t, `fixture teardown failed\n--- FAIL: TestPanickingTestSuite `, out)
}

func (s *HoldsTestSuite) TestAPanickingTestReleasesTheFixture(t *gotest.T) {
	code, out, events := s.holds(t, []string{"HOLDS_PANIC=1"}, "-run", "TestPanickingTestSuite")
	gotest.NotEqual(t, 0, code, "exit 0 after a panic\n%s", out)
	gotest.Contains(t, out, "panic: method panic")
	gotest.Equal(t, []string{"setup", "teardown"}, events)
}

// The generated guard fails a panicking sub-benchmark, so the parent's
// cleanups, the fixture teardown among them, run as after any failure.
func (s *HoldsTestSuite) TestAPanickingBenchmarkReleasesTheFixture(t *gotest.T) {
	code, out, events := s.holds(t, []string{"HOLDS_BENCH_PANIC=1"}, "-run", "^$", "-bench", "BenchmarkPanickingTestSuite")
	gotest.Equal(t, 1, code, "a panicking benchmark is a failed benchmark\n%s", out)
	gotest.Contains(t, out, "panic: benchmark panic")
	gotest.Contains(t, out, "--- FAIL: BenchmarkPanickingTestSuite/BenchmarkPanics")
	gotest.Regexp(t, `\.BenchmarkPanics\(`, out, "the traceback starts where the panic did")
	gotest.Equal(t, []string{"setup", "teardown"}, events)
}

// The testing package can lose a benchmark's panic: it signals the benchmark
// done while the panic unwinds, and the run may print PASS and exit 0 first.
// Wherever the panic happens on the benchmark path, gotest fails the
// benchmark instead, so the verdict never depends on that race.
func (s *HoldsTestSuite) TestABenchmarkPanicFailsTheBenchmark(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc     string
		scenario string
		bench    string
		value    string
		failed   string
		events   []string
	}{
		{"a sub-benchmark of a fixture-bound suite", "benchpanic", "BenchmarkBenchPanicTestSuite", "benchmark panic", "BenchmarkBenchPanicTestSuite/BenchmarkPanics", []string{"setup", "teardown"}},
		{"a sub-benchmark of a suite without fixtures", "unboundbench", "BenchmarkUnboundBenchTestSuite", "unbound benchmark panic", "BenchmarkUnboundBenchTestSuite/BenchmarkPanics", nil},
		{"BeforeAll", "benchbeforeall", "BenchmarkBenchSetupTestSuite", "bench BeforeAll panic", "BenchmarkBenchSetupTestSuite", []string{"setup", "teardown"}},
		{"AfterAll", "benchafterall", "BenchmarkBenchTeardownTestSuite", "bench AfterAll panic", "BenchmarkBenchTeardownTestSuite", []string{"setup", "teardown"}},
	}) {
		code, out, events := goTest(t, s.root, "./testdata/panics/", "PANICS_LOG", []string{"PANICS=" + tc.scenario}, "-run", "^$", "-bench", "^"+tc.bench+"$", "-benchtime=1x")
		gotest.Equal(t, 1, code, out)
		gotest.Contains(t, out, "panic: "+tc.value)
		gotest.Contains(t, out, "--- FAIL: "+tc.failed, out)
		gotest.Equal(t, tc.events, events, out)
	}
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
		code, out, events := goTest(t, s.root, "./testdata/panics/", "PANICS_LOG", []string{"PANICS=" + tc.scenario}, tc.args...)
		gotest.NotEqual(t, 0, code, out)
		gotest.Contains(t, out, tc.value)
		gotest.Equal(t, []string{"setup", "teardown"}, events, out)
	}
}
