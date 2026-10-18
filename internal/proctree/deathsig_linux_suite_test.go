//go:build linux

package proctree_test

import (
	"context"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeathSignalTestSuite: a child started through a tree is asked to stop when
// its parent is gone, whatever group it leads, so it can release what it
// holds before it exits.
type DeathSignalTestSuite struct{}

func (s *DeathSignalTestSuite) TestAChildTearsDownWhenItsParentDies(t *gotest.T) {
	t.When("the parent is killed while a child it started through a tree listens", func(w *gotest.T) {
		marker := filepath.Join(w.TempDir(), "torn-down")
		c := start(w, context.Background(), "parent", envGrandMarker+"="+marker)
		gotest.Len(w, c.pids, 1)
		exited, stop, err := watchExit(c.pids[0])
		gotest.NoError(w, err)
		defer stop()
		defer func() { _ = syscall.Kill(c.pids[0], syscall.SIGKILL) }() // never leave it behind when the check fails
		gotest.NoError(w, syscall.Kill(c.cmd.Process.Pid, syscall.SIGKILL))
		c.wait(w)

		w.It("the child tears down, then is gone", func(it *gotest.T) {
			gotest.Eventually(it, exitWithin, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, exited(), "the child outlived its parent")
			})
			gotest.True(it, exists(marker), "the child was killed without a chance to tear down")
		})
	})
}
