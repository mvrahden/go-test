//go:build unix

package testkit

import (
	"errors"
	"os"
	"syscall"
)

// processAlive reports whether a process with pid is running.
func processAlive(pid int) bool {
	// 0 and -1 would signal a group, not a process.
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
