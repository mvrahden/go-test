package drill

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Scratch is the directory a drill copies and builds in. Its path is fixed
// per checkout and shard: Go keys a main-module package's build on its
// directory, so trees at the same paths reuse the build cache across runs.
type Scratch struct {
	Dir  string
	lock *os.File
}

// ScratchRoot is where the scratch directories live.
func ScratchRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gotest", "drill"), nil
}

// OpenScratch locks and empties checkout's scratch for shard i of n under
// root. A second drill on the same checkout and shard is refused at once.
func OpenScratch(root, checkout string, i, n int) (*Scratch, error) {
	real, err := realPath(checkout)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(real))
	key := fmt.Sprintf("%s-%d-of-%d", hex.EncodeToString(sum[:6]), i, n)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(root, key+".lock")
	lock, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLock(lock); err != nil {
		_ = lock.Close()
		if errors.Is(err, errLocked) {
			return nil, fmt.Errorf("another drill is running on %s as shard %d/%d (lock %s); wait for it or stop it", real, i, n, path)
		}
		return nil, err
	}
	s := &Scratch{Dir: filepath.Join(root, key), lock: lock}
	// A drill killed outright leaves its trees; they are cleared here.
	if err := s.Clear(); err != nil {
		_ = s.Release()
		return nil, err
	}
	if err := os.Mkdir(s.Dir, 0o700); err != nil {
		_ = s.Release()
		return nil, err
	}
	return s, nil
}

// Clear removes the scratch directory and everything in it.
func (s *Scratch) Clear() error { return removeAll(s.Dir) }

// Release gives up the lock; the next drill may then open the scratch.
func (s *Scratch) Release() error { return s.lock.Close() }

func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
