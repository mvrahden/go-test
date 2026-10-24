//go:build windows

package proctree_test

import (
	"golang.org/x/sys/windows"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AliveWindowsTestSuite covers the processes Windows will not open.
// Sequential: it swaps the package's process lookup.
type AliveWindowsTestSuite struct{}

func (s *AliveWindowsTestSuite) TestAProcessThisUserMayNotOpen(t *gotest.T) {
	t.It("reports the System process, pid 4, as running", func(it *gotest.T) {
		gotest.True(it, proctree.Alive(4))
	})

	restore := proctree.ExportFailOpenProcess(windows.ERROR_ACCESS_DENIED)
	denied := proctree.Alive(4)
	restore()
	t.It("reports a process it is denied access to as running", func(it *gotest.T) {
		gotest.True(it, denied)
	})

	restore = proctree.ExportFailOpenProcess(windows.ERROR_INVALID_PARAMETER)
	gone := proctree.Alive(4)
	restore()
	t.It("reports a pid no process has as gone", func(it *gotest.T) {
		gotest.False(it, gone)
	})
}
