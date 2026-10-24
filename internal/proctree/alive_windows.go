//go:build windows

package proctree

import (
	"errors"
	"math"

	"golang.org/x/sys/windows"
)

// openProcess opens a process handle; a variable so tests can stand in for
// the answers Windows gives.
var openProcess = windows.OpenProcess

// Alive reports whether a process with pid is running.
func Alive(pid int) bool {
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return false
	}
	h, err := openProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Denied is a process this user may not query, as EPERM on Unix;
		// an invalid parameter is a pid no process has.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	const stillActive = 259
	return exitCode == stillActive
}
