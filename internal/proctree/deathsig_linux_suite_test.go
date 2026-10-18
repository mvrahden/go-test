//go:build linux

package proctree_test

import (
	"context"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeathSignalTestSuite: a child started through a tree dies with its parent,
// whatever group it leads.
type DeathSignalTestSuite struct{}

func (s *DeathSignalTestSuite) TestAChildDiesWithItsParent(t *gotest.T) {
	t.When("the root exits leaving a grandchild that leads a group of its own", func(w *gotest.T) {
		c := start(w, context.Background(), "orphangroup")
		gotest.Len(w, c.pids, 1)
		exited, stop, err := watchExit(c.pids[0])
		gotest.NoError(w, err)
		defer stop()
		defer func() { _ = syscall.Kill(c.pids[0], syscall.SIGKILL) }() // never leave it behind when the check fails
		c.wait(w)

		w.It("the grandchild is gone with its parent", func(it *gotest.T) {
			gotest.Eventually(it, exitWithin, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, exited(), "the grandchild outlived its parent")
			})
		})
	})
}
