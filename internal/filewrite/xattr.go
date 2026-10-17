//go:build linux || darwin || freebsd || netbsd

package filewrite

import (
	"bytes"
	"errors"

	"golang.org/x/sys/unix"
)

// hasXattrs reports whether path has extended attributes a new file would
// not get, ACLs among them. An SELinux label does not count: a new file
// takes its directory's, which the old one almost always has.
func hasXattrs(path string) bool {
	n, err := unix.Listxattr(path, nil)
	if err != nil {
		return !errors.Is(err, unix.ENOTSUP)
	}
	if n == 0 {
		return false
	}
	buf := make([]byte, n)
	if n, err = unix.Listxattr(path, buf); err != nil {
		return true
	}
	for name := range bytes.SplitSeq(bytes.TrimRight(buf[:n], "\x00"), []byte{0}) {
		if string(name) != "security.selinux" {
			return true
		}
	}
	return false
}
