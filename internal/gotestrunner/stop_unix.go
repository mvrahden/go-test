//go:build unix

package gotestrunner

import (
	"fmt"
	"syscall"
)

// signalSuite tells one suite process to stop.
var signalSuite = signalPID

// stopAgain sends a suite process the second stop signal, which releases its
// fixtures at once.
var stopAgain = signalPID

// signalPID sends pid SIGTERM, among the signals the runtime counts once the
// stop file exists. The pid must be in the suite's process group: under a pid
// namespace, or after pid reuse, it names a stranger, and the caller
// interrupts the group instead.
func signalPID(pid, group int) error {
	if err := inGroup(pid, group); err != nil {
		return err
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

// killPID kills pid, which must be in the suite's process group as for
// signalPID.
func killPID(pid, group int) error {
	if err := inGroup(pid, group); err != nil {
		return err
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}

// inGroup reports why pid is not in the suite's process group, if it is not.
func inGroup(pid, group int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return err
	}
	if pgid != group {
		return fmt.Errorf("pid %d is outside the suite's process group %d", pid, group)
	}
	return nil
}
