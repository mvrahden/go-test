//go:build unix

package gotestrunner_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
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
}

// BeforeEach starts a process outside any suite's tree that records a SIGTERM.
func (s *StopSignalTestSuite) BeforeEach(t *gotest.T) *stopCtx {
	dir := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", `trap 'touch "$0"; exit 0' TERM; while :; do sleep 0.05; done`, filepath.Join(dir, "stranger-got-term")) //nolint:gosec // G204: fixed args
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	gotest.NoError(t, cmd.Start())
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	return &stopCtx{dir: dir, stranger: cmd, exited: exited}
}

func (s *StopSignalTestSuite) AfterEach(t *gotest.T, ctx *stopCtx) {
	_ = ctx.stranger.Process.Kill()
	<-ctx.exited
}

func (s *StopSignalTestSuite) TestAPublishedPidOutsideTheTree(t *gotest.T, ctx *stopCtx) {
	stop := filepath.Join(ctx.dir, "stop")
	gotest.NoError(t, os.WriteFile(protocol.StopPIDPath(stop), []byte(strconv.Itoa(ctx.stranger.Process.Pid)), 0o600))
	marker := filepath.Join(ctx.dir, "suite-got-term")
	budget := filepath.Join(ctx.dir, "budget")
	gotest.NoError(t, os.WriteFile(budget, []byte("1s"), 0o600))
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
		Exec:       []string{"/bin/sh", "-c", `trap 'touch "$0"; exit 0' TERM; while :; do sleep 0.05; done`, marker},
		BinaryPath: "suite.test",
		StopFile:   stop,
		BudgetFile: budget,
	}
	run, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := gotestrunner.RunSingleSuite(run, target, nil, false)

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
