//go:build windows

package testkit

import (
	"math"

	"golang.org/x/sys/windows"
)

// processAlive reports whether a process with pid is running.
func processAlive(pid int) bool {
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	const stillActive = 259
	return exitCode == stillActive
}

// zombie: an exited Windows process is not running, whoever holds a handle.
func zombie(int) bool { return false }
