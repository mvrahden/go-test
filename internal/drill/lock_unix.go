//go:build unix

package drill

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

var errLocked = errors.New("locked")

// tryLock takes an exclusive lock on f without waiting for it.
func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return errLocked
	}
	return err
}
