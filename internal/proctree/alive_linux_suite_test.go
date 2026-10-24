//go:build linux

package proctree_test

import (
	"os"
	"runtime/debug"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AliveLinuxTestSuite covers the pidfd Linux opens for every lookup.
// Sequential: it switches the garbage collector off.
type AliveLinuxTestSuite struct{}

func (s *AliveLinuxTestSuite) TestAliveOpensNoFdItKeeps(t *gotest.T) {
	// Off, so no finalizer closes a pidfd Alive left open.
	gc := debug.SetGCPercent(-1)
	before, beforeErr := os.ReadDir("/proc/self/fd")
	for range 100 {
		proctree.Alive(os.Getpid())
	}
	after, afterErr := os.ReadDir("/proc/self/fd")
	debug.SetGCPercent(gc)
	t.It("leaves this process's open fds as they were after 100 lookups", func(it *gotest.T) {
		gotest.NoError(it, beforeErr)
		gotest.NoError(it, afterErr)
		gotest.Less(it, len(after)-len(before), 10)
	})
}
