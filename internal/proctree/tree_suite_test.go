package proctree_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// exitWithin bounds how long a stopped tree may take to exit; on Windows a
// shutdown request starts a helper process first.
const exitWithin = 30 * time.Second

// TreeTestSuite proves a tree is stopped as a whole and alone: a shutdown
// request and a force-kill reach the root and its descendants, and nothing
// outside the tree.
type TreeTestSuite struct{}

func (s *TreeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// child is a started child program and the tree it roots.
type child struct {
	cmd  *exec.Cmd
	tree *proctree.Tree
	pids []int
}

// start runs a child program as the root of a tree and waits until it is ready.
func start(t *gotest.T, ctx context.Context, mode string, env ...string) *child {
	cmd := exec.CommandContext(ctx, os.Args[0]) //nolint:gosec // G204: this test binary
	cmd.Env = append(childEnv(mode), env...)
	tree := proctree.New(cmd)
	out, err := cmd.StdoutPipe()
	gotest.NoError(t, err)
	gotest.NoError(t, tree.Start())
	pids, ready := readUntilReady(bufio.NewScanner(out))
	if !ready {
		_ = tree.Kill()
		_ = cmd.Wait()
		tree.Release()
		gotest.Fail(t, "the %s child never became ready", mode)
	}
	return &child{cmd: cmd, tree: tree, pids: pids}
}

// wait waits for the root to exit and releases the tree.
func (c *child) wait(t *gotest.T) {
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
		c.tree.Release()
	case <-time.After(exitWithin):
		_ = c.tree.Kill()
		<-done
		c.tree.Release()
		gotest.Fail(t, "the root did not exit within %s", exitWithin)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *TreeTestSuite) TestInterrupt(t *gotest.T) {
	t.When("the root and a grandchild listen", func(w *gotest.T) {
		dir := w.TempDir()
		root, grandchild := filepath.Join(dir, "root"), filepath.Join(dir, "grandchild")
		c := start(w, context.Background(), "listen", envMarker+"="+root, envGrandMarker+"="+grandchild)
		err := c.tree.Interrupt()
		c.wait(w)

		w.It("reaches both, and both clean up", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.True(it, exists(root), "the root never saw the request")
			gotest.True(it, exists(grandchild), "the grandchild never saw the request")
		})
	})

	t.When("another tree runs beside it", func(w *gotest.T) {
		caller := make(chan os.Signal, 1)
		signal.Notify(caller, interruptSignals...)
		dir := w.TempDir()
		target, sibling := filepath.Join(dir, "target"), filepath.Join(dir, "sibling")
		other := start(w, context.Background(), "listen", envMarker+"="+sibling)
		c := start(w, context.Background(), "listen", envMarker+"="+target)
		gotest.NoError(w, c.tree.Interrupt())
		c.wait(w)
		// A console control event left pending goes out on the next console
		// call of any process; starting one makes that call.
		probe := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
		probe.Env = childEnv("exit")
		gotest.NoError(w, probe.Run())
		time.Sleep(time.Second)
		signal.Stop(caller)
		siblingInterrupted := exists(sibling)
		gotest.NoError(w, other.tree.Interrupt())
		other.wait(w)

		w.It("reaches neither the other tree nor the caller", func(it *gotest.T) {
			gotest.True(it, exists(target), "the target never saw the request")
			gotest.False(it, siblingInterrupted, "the request reached a sibling tree")
			gotest.Empty(it, caller, "the request reached the caller")
		})
	})

	t.When("the tree was released", func(w *gotest.T) {
		c := start(w, context.Background(), "exit")
		c.wait(w)

		w.It("signals nothing and reports the tree done", func(it *gotest.T) {
			gotest.ErrorIs(it, c.tree.Interrupt(), os.ErrProcessDone)
			gotest.ErrorIs(it, c.tree.Kill(), os.ErrProcessDone)
		})
	})

	t.When("the command never started", func(w *gotest.T) {
		tree := proctree.New(exec.CommandContext(context.Background(), os.Args[0])) //nolint:gosec // G204: this test binary

		w.It("signals nothing", func(it *gotest.T) {
			gotest.NoError(it, tree.Interrupt())
			gotest.NoError(it, tree.Kill())
		})
	})
}

func (s *TreeTestSuite) TestCancel(t *gotest.T) {
	t.When("the command's context is canceled", func(w *gotest.T) {
		marker := filepath.Join(w.TempDir(), "root")
		ctx, cancel := context.WithCancel(context.Background())
		c := start(w, ctx, "listen", envMarker+"="+marker)
		cancel()
		c.wait(w)

		w.It("asks the tree to shut down instead of killing the root", func(it *gotest.T) {
			gotest.True(it, exists(marker), "the root was not asked to shut down")
			gotest.True(it, c.cmd.ProcessState.Success(), "the root did not exit on its own: %v", c.cmd.ProcessState)
		})
	})
}

func (s *TreeTestSuite) TestKill(t *gotest.T) {
	t.When("the root and a grandchild ignore shutdown requests", func(w *gotest.T) {
		c := start(w, context.Background(), "ignore", envSpawn+"=1")
		gotest.Len(w, c.pids, 1)
		exited, stop, err := watchExit(c.pids[0])
		gotest.NoError(w, err)
		killErr := c.tree.Kill()
		c.wait(w)

		w.It("stops both", func(it *gotest.T) {
			gotest.NoError(it, killErr)
			gotest.Eventually(it, exitWithin, 100*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, exited(), "the grandchild survived")
			})
		})
		stop()
	})
}
