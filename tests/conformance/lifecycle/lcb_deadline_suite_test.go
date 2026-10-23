package lifecycle_test

import (
	"context"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeadlineNamingTestSuite: an expired --timeout names the tests it cut
// short, and no test that ended before it or never started.
type DeadlineNamingTestSuite struct{}

func (s *DeadlineNamingTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

// runUntilTimeout runs pkg until a short --timeout ends it. A first run with
// LCB_WARM, under which nothing hangs, builds the binaries, so the timeout
// covers the suites alone.
func runUntilTimeout(ctx context.Context, dir, pkg string, args ...string) (testkit.Run, map[string]bool, error) {
	if _, err := testkit.RunPackageWithEnv(ctx, dir, []string{"LCB_WARM=1"}, pkg, args...); err != nil {
		return testkit.Run{}, nil, err
	}
	run, err := testkit.RunPackage(ctx, dir, pkg, append([]string{"--timeout=10s"}, args...)...)
	named := map[string]bool{}
	for _, ev := range run.Events {
		if ev.Action == "output" && strings.Contains(ev.Output, "--timeout expired while this test was running") {
			named[ev.Test] = true
		}
	}
	return run, named, err
}

func (s *DeadlineNamingTestSuite) TestACrashBeforeTheTimeout(t *gotest.T) {
	run, named, err := runUntilTimeout(t.Context(), t.TempDir(), "../testdata/lcbcrash/")
	gotest.NoError(t, err)

	t.When("one suite's process crashed on its own and another suite hangs until --timeout", func(w *gotest.T) {
		w.It("names only the hung test as running when the timeout expired", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestCrashTestSuite/TestCrash"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, map[string]bool{"TestHangTestSuite/TestHang": true}, named, "stdout: %s", run.Stdout)
		})
	})
}

func (s *DeadlineNamingTestSuite) TestATestWaitingForAParallelSlot(t *gotest.T) {
	run, named, err := runUntilTimeout(t.Context(), t.TempDir(), "../testdata/lcbqueue/", "-parallel=1")
	gotest.NoError(t, err)

	t.When("parallel methods wait for the one slot -parallel=1 leaves and the method holding it hangs until --timeout", func(w *gotest.T) {
		w.It("names only the method holding the slot, not those still waiting for it", func(it *gotest.T) {
			gotest.Len(it, named, 1, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.False(it, named["TestQueueTestSuite/TestQuick"], "stdout: %s", run.Stdout)
		})
	})
}

func (s *DeadlineNamingTestSuite) TestAHookThatHangs(t *gotest.T) {
	run, named, err := runUntilTimeout(t.Context(), t.TempDir(), "../testdata/lcbhook/")
	gotest.NoError(t, err)

	t.When("one suite hangs in its BeforeAll and another in a test method until --timeout", func(w *gotest.T) {
		w.It("names the first suite itself and the second suite's method", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{"TestHookTestSuite": true, "TestMethodTestSuite/TestHang": true}, named, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
}
