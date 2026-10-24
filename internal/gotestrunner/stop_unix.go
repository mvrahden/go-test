//go:build unix

package gotestrunner

import (
	"fmt"
	"syscall"
)

// signalSuite tells one suite process to stop.
var signalSuite = signalPID

// signalPID sends pid SIGTERM, among the signals the runtime counts once the
// stop file exists. The pid must be in the suite's process group: under a pid
// namespace, or after pid reuse, it names a stranger, and the caller
// interrupts the group instead.
func signalPID(pid, group int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return err
	}
	if pgid != group {
		return fmt.Errorf("pid %d is outside the suite's process group %d", pid, group)
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
