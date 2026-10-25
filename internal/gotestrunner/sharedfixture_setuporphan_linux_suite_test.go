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

// envHungSetupStandInRunner makes a run of this test binary a runner that
// starts the slow-setup shared fixture in the named directory, prints the
// fixture process's pid once its setup has begun and stays, until it is
// killed.
const envHungSetupStandInRunner = "GOTEST_TEST_HUNG_SETUP_STAND_IN_RUNNER_DIR"

func init() {
	if dir := os.Getenv(envHungSetupStandInRunner); dir != "" {
		os.Exit(hungSetupStandInRunner(dir))
	}
}

func hungSetupStandInRunner(dir string) int {
	proc, err := gotestrunner.StartSharedFixtures(context.Background(), dir, slowSetupFixture(), 0)
	if err != nil {
		return 2
	}
	for !gotestrunner.ExportSetupReported(proc) {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // into BeforeAll
	fmt.Printf("pid=%d\n", gotestrunner.ExportPid(proc))
	time.Sleep(5 * time.Minute)
	return 0
}

// prSetChildSubreaper makes this process the one an orphaned descendant is
// handed to, so the test can read the fixture process's exit status.
const prSetChildSubreaper = 0x24

// SharedFixtureSetupOrphanTestSuite: a shared fixture process whose runner
// dies during a setup that never returns does not live on for good.
// Sequential: the fixture is configured through the environment.
type SharedFixtureSetupOrphanTestSuite struct{}

// SuiteConfig: locally 31.2s, 30.25s of it the budgets waited out; the rest
// seven times slower is 37s, twice that 74s. Not Exclusive: the verdict is a
// lower bound, which a busy machine only makes later.
func (s *SharedFixtureSetupOrphanTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 75 * time.Second
	return cfg
}

func (s *SharedFixtureSetupOrphanTestSuite) TestAHungSetupEndsWithItsBudgetsOnceTheRunnerIsGone(t *gotest.T) {
	// The setup budget is the declared Timeout; the teardown budget adds 30s.
	t.Setenv(fixtures.EnvSlowSetupTimeout, (250 * time.Millisecond).String())
	t.Setenv(fixtures.EnvSlowSetupDelay, (5 * time.Minute).String())
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0); errno != 0 {
		gotest.NoError(t, errno, "becoming the subreaper of the fixture process")
	}
	runner := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	runner.Env = append(os.Environ(), envHungSetupStandInRunner+"="+t.TempDir())
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
		gotest.Fail(t, "the stand-in runner never started the shared fixture's setup")
	}
	gotest.NoError(t, runner.Process.Kill())
	_ = runner.Wait()
	killed := time.Now()
	exited := make(chan syscall.WaitStatus, 1)
	go func() {
		var ws syscall.WaitStatus
		_, _ = syscall.Wait4(pid, &ws, 0, nil)
		exited <- ws
	}()

	t.When("the runner is killed while the fixture's BeforeAll never returns", func(w *gotest.T) {
		w.It("the process exits with status 1 once its setup and teardown budgets have passed", func(it *gotest.T) {
			var ws syscall.WaitStatus
			select {
			case ws = <-exited:
			case <-time.After(60 * time.Second):
				_ = syscall.Kill(pid, syscall.SIGKILL)
				<-exited
				gotest.Fail(it, "the shared fixture process outlived its runner")
			}
			gotest.True(it, ws.Exited(), "the process ended by a signal, not by its own exit")
			gotest.Equal(it, 1, ws.ExitStatus())
			gotest.GreaterOrEqual(it, time.Since(killed), 30*time.Second, "the process gave its setup and teardown less than their budgets")
		})
	})
}
