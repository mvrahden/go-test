//go:build windows

package gotestrunner

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on the first byte of path, creating it,
// and returns the release. A lock whose file was replaced while it waited
// is refused with errLockReplaced.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	lockOpened(path)
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, err
	}
	if !stillAt(f, path) {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
		f.Close()
		return nil, errLockReplaced
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
		f.Close()
	}, nil
}

// tryLockFile is lockFile without waiting: a lock someone holds is an error.
func tryLockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
		f.Close()
	}, nil
}

// releaseEvicted unlocks an evicted binary's lock file, then tries to remove
// it: Windows deletes no file a handle holds open, this one's included, and
// a run that holds it meanwhile keeps it.
func releaseEvicted(lock string, unlock func()) {
	unlock()
	_ = os.Remove(lock)
}
