//go:build unix

package gotestrunner

import "syscall"

// converterAttr starts a converter in a process group of its own: a signal to
// the suite's group, or the terminal's to the CLI's, never reaches it. It gets
// no death signal: once the runner is gone its input ends, and it exits.
func converterAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
