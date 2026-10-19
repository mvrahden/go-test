//go:build aix

package testkit

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// lockFD takes f's exclusive lock. AIX has no flock; a record lock excludes
// other processes only, which is where claims and sweeps race.
func lockFD(f *os.File, try bool) error {
	cmd := unix.F_SETLKW
	if try {
		cmd = unix.F_SETLK
	}
	return unix.FcntlFlock(f.Fd(), cmd, &unix.Flock_t{Type: unix.F_WRLCK, Whence: io.SeekStart})
}

func unlockFD(f *os.File) {
	_ = unix.FcntlFlock(f.Fd(), unix.F_SETLK, &unix.Flock_t{Type: unix.F_UNLCK, Whence: io.SeekStart})
}

func openLock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // G304: the lock file of a gotest-cli- temp dir
}
