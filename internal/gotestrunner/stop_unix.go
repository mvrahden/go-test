//go:build unix

package gotestrunner

import "syscall"

// signalSuite tells one suite process to stop. SIGTERM is among the signals
// the runtime counts once the stop file exists.
func signalSuite(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }
