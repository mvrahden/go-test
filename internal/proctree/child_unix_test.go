//go:build !windows

package proctree_test

import (
	"errors"
	"os"
	"syscall"
)

var interruptSignals = []os.Signal{syscall.SIGTERM}

func printConsoleProcesses() int { return 2 }

// watchExit reports whether pid has exited. A process whose parent was killed
// is reaped by init, so it does not linger as a zombie.
func watchExit(pid int) (exited func() bool, stop func(), err error) {
	return func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) }, func() {}, nil
}
