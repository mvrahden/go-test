//go:build windows

package filewrite

import (
	"os"
	"syscall"
)

// renameKeeps reports whether a new file renamed over target keeps what
// target has: no other link to it. Ownership and ACLs are inherited from the
// directory either way.
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
