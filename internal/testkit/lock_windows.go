//go:build windows

package testkit

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFD takes f's exclusive lock: LockFileEx, held per handle, so two opens
// in one process exclude each other too.
func lockFD(f *os.File, try bool) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if try {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, new(windows.Overlapped))
}

func unlockFD(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}

// openLock opens the lock, creating it, sharing delete, so its holder can
// remove it while others wait for it.
func openLock(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, share, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
