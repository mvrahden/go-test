//go:build windows

package filewrite

import (
	"errors"
	"os"
	"syscall"
)

// renameKeeps reports whether a new file renamed over target keeps what
// target has; only a second link is checked. The new file takes the
// directory's inherited ACL and our owner, and has no alternate data
// streams, so an explicit ACL, another owner or a stream is lost.
func renameKeeps(target string, _ os.FileInfo) bool {
	f, err := os.Open(target)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d); err != nil {
		return false
	}
	return d.NumberOfLinks == 1
}

func sameGroup(*os.File, os.FileInfo) bool { return true }

// errNotSameDevice is ERROR_NOT_SAME_DEVICE, which syscall does not name.
const errNotSameDevice syscall.Errno = 17

// renameRefused reports a rename the target's volume refuses.
func renameRefused(err error) bool {
	return errors.Is(err, syscall.EXDEV) || errors.Is(err, errNotSameDevice)
}
