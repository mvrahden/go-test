package gotestrunner

import (
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
)

// GracefulShutdownDelay is the grace a process gets to exit after it is asked
// to shut down when no teardown budget file names one: the fallback for a
// suite binary that wrote none, and the grace of the helper trees below. A
// budget file can name any length; nothing else bounds a managed process.
const GracefulShutdownDelay = 5*time.Minute + 30*time.Second

// runTree runs cmd as the root of its own process tree. A canceled context
// asks the tree to shut down, and GracefulShutdownDelay later kills the root.
func runTree(cmd *exec.Cmd) error {
	tree := proctree.New(cmd)
	cmd.WaitDelay = GracefulShutdownDelay
	defer tree.Release()
	if err := tree.Start(); err != nil {
		return err
	}
	track(tree, liveTree{root: cmd.Process.Pid})
	defer untrack(tree)
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
	case status == raceExitCode:
		// -race reports a race found after the last test — in fixture
		// teardown, which runs after m.Run — by its own exit code.
		return 1, "by the race detector (exit status 66)"
	case proctree.Terminated(status):
		return 1, "by the system (status " + proctree.StatusHex(status) + ")"
	}
	return status, ""
}

// raceExitCode is the race detector's default exit status (GORACE exitcode).
const raceExitCode = 66
