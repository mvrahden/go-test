//go:build linux

package gotestrunner_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// envStandInRunner makes a run of this test binary a runner that starts the
// slow-teardown shared fixture in the named directory, prints the fixture
// process's pid and stays, until it is killed.
const envStandInRunner = "GOTEST_TEST_STAND_IN_RUNNER_DIR"

func init() {
	if dir := os.Getenv(envStandInRunner); dir != "" {
		os.Exit(standInRunner(dir))
	}
}

func standInRunner(dir string) int {
	ctx := context.Background()
	proc, err := gotestrunner.StartSharedFixtures(ctx, dir, slowTeardownFixture(), 30*time.Second)
	if err != nil || proc.WaitAllReady(ctx, 30*time.Second) != nil {
		return 2
	}
	fmt.Printf("pid=%d\n", gotestrunner.ExportPid(proc))
	time.Sleep(5 * time.Minute)
	return 0
}

// SharedFixtureParentGoneTestSuite: a runner that dies leaves no shared
// fixture behind unreleased.
// Sequential: the fixture is configured through the environment.
type SharedFixtureParentGoneTestSuite struct{}

func (s *SharedFixtureParentGoneTestSuite) TestAKilledRunnerLeavesTheFixtureToTearDown(t *gotest.T) {
	marker := filepath.Join(t.TempDir(), "released")
	t.Setenv(fixtures.EnvSlowTeardownDelay, (500 * time.Millisecond).String())
	t.Setenv(fixtures.EnvSlowTeardownMarker, marker)
	runner := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	runner.Env = append(os.Environ(), envStandInRunner+"="+t.TempDir())
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

	t.When("the runner is killed while a shared fixture is up", func(w *gotest.T) {
		w.It("the fixture's AfterAll runs, then its process is gone", func(it *gotest.T) {
			gotest.Eventually(it, 20*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.ErrorIs(poll, syscall.Kill(pid, 0), syscall.ESRCH, "the shared fixture process outlived the runner")
			})
			_, statErr := os.Stat(marker)
			gotest.NoError(it, statErr, "the shared fixture process was gone without its AfterAll")
		})
	})
}
