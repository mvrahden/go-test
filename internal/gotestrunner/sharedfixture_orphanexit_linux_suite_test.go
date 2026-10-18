//go:build linux

package gotestrunner_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// envHungStandInRunner makes a run of this test binary a runner that starts
// the budgeted-teardown shared fixture in the named directory, prints the
// fixture process's pid and stays, until it is killed.
const envHungStandInRunner = "GOTEST_TEST_HUNG_STAND_IN_RUNNER_DIR"

func init() {
	if dir := os.Getenv(envHungStandInRunner); dir != "" {
		os.Exit(hungStandInRunner(dir))
	}
}

func hungStandInRunner(dir string) int {
	ctx := context.Background()
	proc, err := gotestrunner.StartSharedFixtures(ctx, dir, budgetedTeardownFixture(), 30*time.Second)
	if err != nil || proc.WaitAllReady(ctx, 30*time.Second) != nil {
		return 2
	}
	fmt.Printf("pid=%d\n", gotestrunner.ExportPid(proc))
	time.Sleep(5 * time.Minute)
	return 0
}

// SharedFixtureOrphanExitTestSuite: a shared fixture process whose runner is
// gone does not outlive its teardown budget, whatever its AfterAll does.
// Sequential: the fixture is configured through the environment.
type SharedFixtureOrphanExitTestSuite struct{}

// SuiteConfig: Exclusive, as every suite that builds and runs a real shared
// fixture subprocess is; the Timeout covers the 30s budget headroom.
func (s *SharedFixtureOrphanExitTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	cfg.Timeout = 2 * time.Minute
	return cfg
}

func (s *SharedFixtureOrphanExitTestSuite) TestAHungTeardownEndsWithTheBudgetOnceTheRunnerIsGone(t *gotest.T) {
	// The process reports a budget of the declared Timeout plus 30s.
	t.Setenv(fixtures.EnvBudgetedTeardownTimeout, (250 * time.Millisecond).String())
	t.Setenv(fixtures.EnvBudgetedTeardownDelay, (5 * time.Minute).String())
	runner := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	runner.Env = append(os.Environ(), envHungStandInRunner+"="+t.TempDir())
	out, err := runner.StdoutPipe()
	gotest.NoError(t, err)
	gotest.NoError(t, runner.Start())
	pid := 0
	for sc := bufio.NewScanner(out); pid == 0 && sc.Scan(); {
		if v, ok := strings.CutPrefix(sc.Text(), "pid="); ok {
			pid, _ = strconv.Atoi(v)
		}
	}
	if pid == 0 {
		_ = runner.Process.Kill()
		_ = runner.Wait()
		gotest.Fail(t, "the stand-in runner never started the shared fixture")
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }() // never leave it behind when the check fails
	gotest.NoError(t, runner.Process.Kill())
	_ = runner.Wait()
	killed := time.Now()

	t.When("the runner is killed and the fixture's AfterAll never returns", func(w *gotest.T) {
		w.It("the process exits once the reported teardown budget has passed", func(it *gotest.T) {
			gotest.Eventually(it, 50*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
				gotest.ErrorIs(poll, syscall.Kill(pid, 0), syscall.ESRCH, "the shared fixture process outlived the runner")
			})
			gotest.GreaterOrEqual(it, time.Since(killed), 30*time.Second, "the process gave its AfterAll less than the budget")
		})
	})
}
