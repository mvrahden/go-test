//go:build unix

package gotestrunner

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on path, creating it, and
// returns the release. A lock whose file was replaced while it waited is
// refused with errLockReplaced.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	lockOpened(path)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	if !stillAt(f, path) {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		return nil, errLockReplaced
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
