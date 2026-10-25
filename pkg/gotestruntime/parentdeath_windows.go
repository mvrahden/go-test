//go:build windows

package gotestruntime

import (
	"sync/atomic"
	"syscall"
)

// parentHandle is the process that started this one, under the runner only;
// zero otherwise. Windows does not reparent an orphan, so only a handle tells
// that the parent ended. It is opened during init, while the parent still
// holds its pid.
var parentHandle = openParent()

var parentGone atomic.Bool

const (
	errInvalidParameter            = syscall.Errno(87) // no process has the pid
	processQueryLimitedInformation = 0x1000
)

func openParent() syscall.Handle {
	if stopFile == "" {
		return 0
	}
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE|processQueryLimitedInformation, false, uint32(parentPID)) //nolint:gosec // G115: a Windows pid is a DWORD
	if err != nil {
		if err == errInvalidParameter {
			parentGone.Store(true)
			return 0
		}
		h, err = syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(parentPID)) //nolint:gosec // G115: a Windows pid is a DWORD
		if err != nil {
			return 0
		}
		return h
	}
	if startedAfterThis(h) {
		// The parent ended before init, and a newer process took its pid.
		_ = syscall.CloseHandle(h)
		parentGone.Store(true)
		return 0
	}
	return h
}

// startedAfterThis reports whether h was created after this process.
func startedAfterThis(h syscall.Handle) bool {
	var theirs, ours, exit, kernel, user syscall.Filetime
	self, err := syscall.GetCurrentProcess()
	if err != nil ||
		syscall.GetProcessTimes(h, &theirs, &exit, &kernel, &user) != nil ||
		syscall.GetProcessTimes(self, &ours, &exit, &kernel, &user) != nil {
		return false
	}
	return theirs.Nanoseconds() > ours.Nanoseconds()
}

// parentExited reports whether the process that started this one has ended:
// the watcher's poll asks, as no death signal comes here.
func parentExited() bool {
	if parentGone.Load() {
		return true
	}
	if parentHandle == 0 {
		return false
	}
	if ev, err := syscall.WaitForSingleObject(parentHandle, 0); err == nil && ev == syscall.WAIT_OBJECT_0 {
		parentGone.Store(true)
	}
	return parentGone.Load()
}
