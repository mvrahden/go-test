//go:build windows

package gotestrunner

import (
	"math"

	"golang.org/x/sys/windows"
)

func processAlive(pid int) bool {
	// Windows pids are positive uint32s; anything else names no process.
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	// A failed close of a query-only handle changes nothing about the answer.
	defer func() { _ = windows.CloseHandle(h) }()
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	const stillActive = 259
	return exitCode == stillActive
}
