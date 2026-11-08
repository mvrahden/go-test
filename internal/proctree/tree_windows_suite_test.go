//go:build windows

package proctree_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// WindowsTreeTestSuite covers what a Windows tree adds: its own console, which
// scopes a shutdown request, and a job, which outlives its root.
type WindowsTreeTestSuite struct{}

func (s *WindowsTreeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *WindowsTreeTestSuite) TestConsole(t *gotest.T) {
	t.When("a tree starts", func(w *gotest.T) {
		c := start(w, context.Background(), "consoles")
		c.wait(w)

		w.It("runs on a console the caller is not attached to", func(it *gotest.T) {
			gotest.Contains(it, c.pids, c.cmd.Process.Pid, "the root has no console of its own")
			gotest.NotContains(it, c.pids, os.Getpid())
		})
	})
}

func (s *WindowsTreeTestSuite) TestRelease(t *gotest.T) {
	t.When("the root exited and left a process running", func(w *gotest.T) {
		c := start(w, context.Background(), "orphan")
		gotest.Len(w, c.pids, 1)
		exited, stop, err := watchExit(c.pids[0])
		gotest.NoError(w, err)
		c.wait(w)

		w.It("kills that process", func(it *gotest.T) {
			gotest.Eventually(it, exitWithin, 100*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, exited(), "the orphan survived the release")
			})
		})
		stop()
	})
}

func (s *WindowsTreeTestSuite) TestHoldJobWithRoot(t *gotest.T) {
	t.When("the root holds the job and the caller lets go of it", func(w *gotest.T) {
		cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
		cmd.Env = append(childEnv("ignore"), envSpawn+"=1")
		tree := proctree.New(cmd)
		tree.HoldJobWithRoot()
		out, err := cmd.StdoutPipe()
		gotest.NoError(w, err)
		gotest.NoError(w, tree.Start())
		pids, ready := readUntilReady(bufio.NewScanner(out))
		exited, stop := func() bool { return false }, func() {}
		var watchErr error
		heldOpen := false
		if ready && len(pids) == 1 {
			exited, stop, watchErr = watchExit(pids[0])
		}
		if ready && len(pids) == 1 && watchErr == nil {
			// The caller's handles close while the root runs, as when it dies.
			tree.Release()
			time.Sleep(time.Second)
			heldOpen = !exited()
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		tree.Release()
		gotest.True(w, ready, "the root never became ready")
		gotest.Len(w, pids, 1)
		gotest.NoError(w, watchErr)

		w.It("keeps the tree alive while the root lives", func(it *gotest.T) {
			gotest.True(it, heldOpen, "the grandchild died with the caller's handle")
		})
		w.It("kills the tree once the root exits", func(it *gotest.T) {
			gotest.Eventually(it, exitWithin, 100*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, exited(), "the grandchild outlived the root")
			})
		})
		stop()
	})
}

func (s *WindowsTreeTestSuite) TestInterruptConsole(t *gotest.T) {
	t.When("the process has no console", func(w *gotest.T) {
		w.It("reports it done", func(it *gotest.T) {
			gotest.ErrorIs(it, proctree.ExportInterruptConsole(0x7FFFFFF0), os.ErrProcessDone)
		})
	})
}
