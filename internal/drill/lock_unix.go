//go:build unix

package drill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

var errLocked = errors.New("locked")

// tryLock takes an exclusive lock on f without waiting for it.
func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	return lockError(f.Name(), err)
}

// lockError names what a failed flock of path means.
func lockError(path string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EWOULDBLOCK):
		return errLocked
	case errors.Is(err, unix.ENOLCK), errors.Is(err, unix.EOPNOTSUPP):
		return fmt.Errorf("cannot lock %s: its filesystem cannot lock files (%w); point XDG_CACHE_HOME at a local disk", path, err)
	}
	return fmt.Errorf("cannot lock %s: %w", path, err)
}

// ours reports whether this user owns info's file.
func ours(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
