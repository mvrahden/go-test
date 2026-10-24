//go:build unix

package gotestrunner_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StopSignalTestSuite covers whom a cancelled run signals: the suite process
// by the pid it published, and only when that pid is in the suite's tree.
type StopSignalTestSuite struct{}

func (s *StopSignalTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type stopCtx struct {
	dir      string
	stranger *exec.Cmd
	exited   chan struct{}
	leftover int // a process a scenario's suite left running and the run did not end
}

// BeforeEach starts a process outside any suite's tree that records a SIGTERM
// and answers a SIGWINCH.
func (s *StopSignalTestSuite) BeforeEach(t *gotest.T) *stopCtx {
	dir := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", `trap 'touch "$0"; exit 0' TERM; trap 'touch "$0.probe"' WINCH; while :; do sleep 0.05; done`, filepath.Join(dir, "stranger-got-term")) //nolint:gosec // G204: fixed args
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	gotest.NoError(t, cmd.Start())
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	return &stopCtx{dir: dir, stranger: cmd, exited: exited}
}

// settled reports, within a bound, once the stranger has acted on every
// signal sent before the call. The shell runs pending traps in signal order,
// so a SIGTERM already sent ends it before it answers the later SIGWINCH.
func (ctx *stopCtx) settled(within time.Duration) bool {
	_ = ctx.stranger.Process.Signal(syscall.SIGWINCH)
	for deadline := time.Now().Add(within); time.Now().Before(deadline); {
		select {
		case <-ctx.exited:
			return true
		case <-time.After(20 * time.Millisecond):
		}
		if _, err := os.Stat(filepath.Join(ctx.dir, "stranger-got-term.probe")); err == nil {
			return true
		}
	}
	return false
}

func (s *StopSignalTestSuite) AfterEach(t *gotest.T, ctx *stopCtx) {
	_ = ctx.stranger.Process.Kill()
	<-ctx.exited
	if ctx.leftover > 0 {
		_ = syscall.Kill(ctx.leftover, syscall.SIGKILL)
	}
}

func (s *StopSignalTestSuite) TestAPublishedPidOutsideTheTree(t *gotest.T, ctx *stopCtx) {
	stop := filepath.Join(ctx.dir, "stop")
	gotest.NoError(t, os.WriteFile(protocol.StopPIDPath(stop), []byte(strconv.Itoa(ctx.stranger.Process.Pid)), 0o600))
	marker := filepath.Join(ctx.dir, "suite-got-term")
	budget := filepath.Join(ctx.dir, "budget")
	gotest.NoError(t, os.WriteFile(budget, []byte("1s"), 0o600))
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
		Exec:       []string{"/bin/sh", "-c", `trap 'touch "$0"; exit 0' TERM; touch "$0.ready"; while :; do sleep 0.05; done`, marker},
		BinaryPath: "suite.test",
		StopFile:   stop,
		BudgetFile: budget,
	}
	started := time.Now()
	result := gotestrunner.RunSingleSuite(cancelOnceReady(marker+".ready"), target, nil, false)
	gotest.True(t, ctx.settled(10*time.Second), "the stranger did not answer its probe")

	t.When("the run is cancelled and the pid file names a process outside the suite's tree", func(w *gotest.T) {
		w.It("leaves that process alone", func(it *gotest.T) {
			_, err := os.Stat(filepath.Join(ctx.dir, "stranger-got-term"))
			gotest.True(it, os.IsNotExist(err), "the stranger was sent SIGTERM")
		})
		w.It("interrupts the suite's own tree instead", func(it *gotest.T) {
			gotest.True(it, result.CutShort)
			_, err := os.Stat(marker)
			gotest.NoError(it, err, "the suite process was not interrupted")
			gotest.Less(it, time.Since(started), 10*time.Second)
		})
	})
}

// cancelOnceReady returns a context canceled once the file ready exists.
func cancelOnceReady(ready string) context.Context {
	run, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(ready); err == nil {
				return
			}
		}
	}()
	return run
}

func (s *StopSignalTestSuite) TestAStopByPidEndsWhatTheSuiteLeft(t *gotest.T, ctx *stopCtx) {
	stop := filepath.Join(ctx.dir, "stop")
	marker := filepath.Join(ctx.dir, "suite")
	budget := filepath.Join(ctx.dir, "budget")
	gotest.NoError(t, os.WriteFile(budget, []byte("30s"), 0o600))
	// The suite starts a process of its own, publishes its pid and ends on
	// the SIGTERM, leaving that process running.
	target := gotestrunner.SuiteTarget{
		SuiteSpec: gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
		Exec: []string{"/bin/sh", "-c", `sleep 120 & echo $! > "$0.leftover"
echo $$ > "$GOTEST_STOP_FILE.pid"
trap 'exit 0' TERM
touch "$0.ready"
while :; do sleep 0.05; done`, marker},
		BinaryPath: "suite.test",
		StopFile:   stop,
		BudgetFile: budget,
	}
	started := time.Now()
	result := gotestrunner.RunSingleSuite(cancelOnceReady(marker+".ready"), target, nil, false)
	took := time.Since(started)
	data, err := os.ReadFile(marker + ".leftover")
	gotest.NoError(t, err)
	leftover, err := strconv.Atoi(strings.TrimSpace(string(data)))
	gotest.NoError(t, err)
	gone := testkit.EndedWithin(leftover, 2*time.Second)
	if !gone {
		ctx.leftover = leftover
	}

	t.When("the run is cancelled and the suite, stopped by its pid, leaves a process it started running", func(w *gotest.T) {
		w.It("ends that process before the run returns", func(it *gotest.T) {
			gotest.True(it, result.CutShort)
			gotest.True(it, gone, "the process the suite started outlived the run")
		})
		w.It("does not wait out the teardown budget for it", func(it *gotest.T) {
			gotest.Less(it, took, 20*time.Second)
		})
	})
}
