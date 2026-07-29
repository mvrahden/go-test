package gotestgen_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// childTimeout is the -test.timeout of the compiled suite binary. If suite
// cleanup ever waits on work that is itself blocked behind that cleanup, the
// child dies here with "test timed out" instead of hanging this package.
const childTimeout = 15 * time.Second

// childWallClock bounds the whole subprocess, so even a child that ignores its
// own timeout cannot stall the parent run.
const childWallClock = 90 * time.Second

// ParallelLifecycleTestSuite compiles and runs harnesses produced by the real
// generator for suites configured with SuiteConfig{Parallel: true}.
//
// It guards the deadlock fixed in v1.25.x: the generated parent cleanup used to
// wg.Wait() on the parallel test methods, but Go's testing package runs every
// ancestor's cleanup from the goroutine that is panicking, while the test method
// that would release the barrier is still parked inside t.Run. The two waited on
// each other until the outer -timeout alarm shot the process.
type ParallelLifecycleTestSuite struct{}

func (s *ParallelLifecycleTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute}
}

type childRun struct {
	output  string
	elapsed time.Duration
	passed  bool
}

// runGeneratedSuite renders the harness for a testdata package, drops it next to
// the sources inside the throwaway module and runs `go test` over it.
func runGeneratedSuite(t *gotest.T, name string) childRun {
	dir := gotestgen.ExportTestPkgDir(t.T(), name)
	pkg := gotestgen.ExportMustTestPkg(t.T(), name)

	source, _ := renderTestPkg(t.T(), pkg)
	gotest.NotContains(t, source, "sync.WaitGroup", "generated parallel harness must not gate cleanup on a WaitGroup")

	harness := filepath.Join(dir, "gotest_psuite_test.go")
	gotest.NoError(t, os.WriteFile(harness, []byte(source), 0o600))

	// Compile and run as two steps. A combined `go test` would fold build time
	// into the measurement even though -timeout only governs execution, and a
	// compile error would be indistinguishable from a failing suite.
	bin := filepath.Join(t.TempDir(), "suite.test")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	buildArgs := []string{"test", "-c", "-o", bin}
	if raceEnabled {
		buildArgs = append(buildArgs, "-race")
	}
	buildArgs = append(buildArgs, ".")

	build := exec.Command("go", buildArgs...) //nolint:gosec // G204: go tool with test-controlled arguments
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	buildOut, buildErr := build.CombinedOutput()
	gotest.NoError(t, buildErr, "compiling the generated harness failed:\n%s", buildOut)

	ctx, cancel := context.WithTimeout(context.Background(), childWallClock)
	defer cancel()

	// -test.v keeps the suite's own stdout markers even when the child passes.
	cmd := exec.CommandContext(ctx, bin, "-test.v", "-test.count=1", "-test.timeout="+childTimeout.String()) //nolint:gosec // G204: freshly compiled test binary
	cmd.Dir = dir

	start := time.Now()
	out, err := cmd.CombinedOutput()
	run := childRun{output: string(out), elapsed: time.Since(start), passed: err == nil}

	gotest.NoError(t, ctx.Err(), "child never exited on its own:\n%s", run.output)
	return run
}

// assertNoDeadlock is the core regression assertion: the child must report its
// failure by itself, not be cut down by the test-run alarm.
func assertNoDeadlock(t *gotest.T, run childRun) {
	gotest.NotContains(t, run.output, "test timed out",
		"suite cleanup deadlocked; the run was terminated by the -timeout alarm:\n%s", run.output)
	gotest.Less(t, run.elapsed, childTimeout,
		"child ran for %s, at or beyond its own -test.timeout of %s:\n%s", run.elapsed, childTimeout, run.output)
}

func (s *ParallelLifecycleTestSuite) TestParallelSuccess(t *gotest.T) {
	t.When("every parallel test method succeeds", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_ParallelSuccess")

		w.It("passes without deadlocking", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.True(it, run.passed, "expected the generated suite to pass:\n%s", run.output)
		})

		w.It("lets every parallel test method complete", func(it *gotest.T) {
			for _, marker := range []string{"MARK:done alpha", "MARK:done beta", "MARK:done gamma"} {
				gotest.Contains(it, run.output, marker, "a started test method did not complete:\n%s", run.output)
			}
		})

		w.It("runs AfterAll exactly once, after all of them", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(run.output, "MARK:afterall"), "AfterAll must run exactly once:\n%s", run.output)
			gotest.Contains(it, run.output, "calls= 1", "AfterAll must observe its own first invocation:\n%s", run.output)

			afterAll := strings.Index(run.output, "MARK:afterall")
			for _, marker := range []string{"MARK:done alpha", "MARK:done beta", "MARK:done gamma"} {
				gotest.Less(it, strings.Index(run.output, marker), afterAll,
					"AfterAll ran before %q:\n%s", marker, run.output)
			}
		})

		w.It("hands AfterAll a live context", func(it *gotest.T) {
			gotest.Contains(it, run.output, "ctxErr= <nil>",
				"AfterAll received an already-canceled context; the configured SetupTimeout never applies:\n%s", run.output)
		})
	})
}

func (s *ParallelLifecycleTestSuite) TestPanicInParallelMethod(t *gotest.T) {
	t.When("a parallel test method panics while a sibling is in flight", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_ParallelPanic")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "a panicking suite must fail:\n%s", run.output)
		})

		w.It("reports the original panic", func(it *gotest.T) {
			gotest.Contains(it, run.output, "boom from a parallel test method", "the panic value should reach the output:\n%s", run.output)
		})

		w.It("still runs AfterAll exactly once", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(run.output, "MARK:afterall"), "AfterAll must run exactly once:\n%s", run.output)
		})
	})
}

func (s *ParallelLifecycleTestSuite) TestPanicInNestedBehavior(t *gotest.T) {
	t.When("a nested When/It panics inside a parallel test method", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_ParallelNestedPanic")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "a panicking suite must fail:\n%s", run.output)
		})

		w.It("reports the original panic", func(it *gotest.T) {
			gotest.Contains(it, run.output, "boom from a nested behavior", "the panic value should reach the output:\n%s", run.output)
		})

		w.It("still runs AfterAll exactly once", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(run.output, "MARK:afterall"), "AfterAll must run exactly once:\n%s", run.output)
		})
	})
}

func (s *ParallelLifecycleTestSuite) TestMustWithErrorInNestedBehavior(t *gotest.T) {
	t.When("gotest.Must panics on a non-nil error inside a nested behavior", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_ParallelMustError")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "a panicking suite must fail:\n%s", run.output)
		})

		w.It("reports the wrapped error", func(it *gotest.T) {
			gotest.Contains(it, run.output, "Must: got error", "the Must panic should reach the output:\n%s", run.output)
			gotest.NotContains(it, run.output, "MARK:unreachable", "Must must not return on a non-nil error:\n%s", run.output)
		})
	})
}

func (s *ParallelLifecycleTestSuite) TestAssertionFailureAndSkip(t *gotest.T) {
	t.When("a nested behavior fails an assertion and another method skips", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLifecycle_ParallelAssertionFailure")

		w.It("terminates promptly instead of timing out", func(it *gotest.T) {
			assertNoDeadlock(it, run)
			gotest.False(it, run.passed, "a failing suite must fail:\n%s", run.output)
		})

		w.It("lets the unrelated methods finish", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:done passes", "a passing sibling must still complete:\n%s", run.output)
		})

		w.It("still runs AfterAll exactly once", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(run.output, "MARK:afterall"), "AfterAll must run exactly once:\n%s", run.output)
		})
	})
}
