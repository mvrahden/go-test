//go:build unix && !aix

package testkit

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFD takes f's exclusive lock: flock, held per open file, so two opens
// in one process exclude each other too.
func lockFD(f *os.File, try bool) error {
	how := unix.LOCK_EX
	if try {
		how |= unix.LOCK_NB
	}
	return unix.Flock(int(f.Fd()), how) //nolint:gosec // G115: a file descriptor fits an int
}

func unlockFD(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) } //nolint:gosec // G115: a file descriptor fits an int

func openLock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // G304: the lock of a gotest-cli- temp dir
}
