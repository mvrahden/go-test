//go:build windows

package gotestrunner

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// converterAttr starts a converter with no console: no console control event,
// sent to the suite's console or to the CLI's, ever reaches it. Once the
// runner is gone its input ends, and it exits.
func converterAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
}
