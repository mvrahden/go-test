package gotestrunner

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
)

// GracefulShutdownDelay is the grace a process gets to exit after it is asked
// to shut down when no teardown budget file names one: the fallback for a
// suite binary that wrote none, and the grace of the helper trees below. A
// budget file can name more (see WaitDelayCeiling).
const GracefulShutdownDelay = 5*time.Minute + 30*time.Second

// WaitDelayCeiling bounds cmd.Wait on a process that is gone, or was asked to
// go, while a detached grandchild keeps its output pipes open. exec kills the
// process itself when it passes, so it sits strictly above the largest budget
// the runtime writes: a 5m fixture path plus a 5m AfterAll plus its 30s
// margin. Below that, exec's kill would cut a budgeted teardown short.
const WaitDelayCeiling = 11 * time.Minute

// runTree runs cmd as the root of its own process tree. A canceled context
// asks the tree to shut down, and GracefulShutdownDelay later kills the root.
func runTree(cmd *exec.Cmd) error {
	tree := proctree.New(cmd)
	cmd.WaitDelay = GracefulShutdownDelay
	defer tree.Release()
	if err := tree.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// ExitStatusVerdict reads a child's exit status as a verdict. A status the
// child cannot have chosen — a signal on Unix, where os/exec reports -1, or a
// Windows termination status such as 0xC000013A, the one a console interrupt
// leaves — is no verdict at all: the caller reports it as a failure and names
// how the child went. An empty how means the status is the child's own.
func ExitStatusVerdict(status int) (code int, how string) {
	switch {
	case status < 0:
		return 1, "by signal"
	case status > maxExitCode:
		return 1, fmt.Sprintf("by the system (status 0x%X)", uint32(status)) //nolint:gosec // G115: a Windows exit status is a DWORD
	}
	return status, ""
}

// maxExitCode is the highest status a program sets as its own exit code.
// Windows allows any 32-bit value, but a termination status uses the high bits
// (0xC000013A), so anything above the conventional range is a termination.
const maxExitCode = 255
