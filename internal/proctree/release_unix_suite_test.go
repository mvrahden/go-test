//go:build unix

package proctree_test

import (
	"context"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReleaseTestSuite: releasing a tree whose root has exited leaves none of
// the root's group behind.
type ReleaseTestSuite struct{}

func (s *ReleaseTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.DefaultSuiteConfig() // 1s locally, cold under -race on 4 cores
}

func (s *ReleaseTestSuite) TestReleaseKillsTheGroup(t *gotest.T) {
	t.When("the root exits leaving an ignoring grandchild in its group", func(w *gotest.T) {
		c := start(w, context.Background(), "orphan")
		gotest.Len(w, c.pids, 1)
		exited, stop, err := watchExit(c.pids[0])
		gotest.NoError(w, err)
		defer stop()
		defer func() { _ = syscall.Kill(c.pids[0], syscall.SIGKILL) }() // never leave it behind when the check fails
		c.wait(w)

		w.It("Release kills the grandchild", func(it *gotest.T) {
			gotest.Eventually(it, exitWithin, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, exited(), "the grandchild outlived the release of its tree")
			})
		})
	})
}
