package drill

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Scratch is the directory a drill copies and builds in. Its path is fixed
// per checkout and shard: Go keys a main-module package's build on its
// directory, so trees at the same paths reuse the build cache across runs.
type Scratch struct {
	Dir  string
	lock *os.File
}

// ScratchRoot is where the scratch directories live: the user's cache
// dir, or a fixed dir under the temp dir when there is none it can write.
func ScratchRoot() (string, error) {
	if base, err := os.UserCacheDir(); err == nil {
		root := filepath.Join(base, "gotest", "drill")
		if os.MkdirAll(root, 0o700) == nil {
			return root, nil
		}
	}
	// Shared with other users: they may not rename the fallback away,
	// plant it, or write in it.
	tmp := os.TempDir()
	if info, err := os.Stat(tmp); err != nil {
		return "", err
	} else if info.Mode().Perm()&0o002 != 0 && info.Mode()&os.ModeSticky == 0 {
		return "", fmt.Errorf("%s is writable by anyone and not sticky", tmp)
	}
	root := filepath.Join(tmp, "gotest-drill-"+strconv.Itoa(os.Getuid()))
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	if info, err := os.Lstat(root); err != nil {
		return "", err
	} else if !info.IsDir() || !ours(info) {
		return "", fmt.Errorf("%s is not a directory of this user's", root)
	} else if info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("%s is writable by others", root)
	}
	return root, nil
}

// unused is how long a scratch no drill opened is kept: a killed drill's
// trees would otherwise outlive reboots.
const unused = 7 * 24 * time.Hour

// OpenScratch locks and empties checkout's scratch for shard i of n under
// root, and prunes the scratches under root unused for a week. A second
// drill on the same checkout and shard is refused at once.
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
	lock, err := lockAt(path, true)
	if err != nil {
		if errors.Is(err, errLocked) {
			return nil, fmt.Errorf("another drill is running on %s as shard %d/%d (lock %s%s); wait for it or stop it", real, i, n, path, heldBy(path))
		}
		return nil, err
	}
	// The lock file's time is when its scratch was last used.
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	prune(root, key)
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

// lockAt opens and locks path, created if create. A lock file pruned
// between the open and the lock is opened again.
func lockAt(path string, create bool) (*os.File, error) {
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	for {
		f, err := os.OpenFile(path, flags, 0o600)
		if err != nil {
			return nil, err
		}
		lockOpened(path)
		err = tryLock(f)
		if err == nil && names(f, path) {
			return f, nil
		}
		// A prune holding the old file is no other drill.
		replaced := (err == nil || errors.Is(err, errLocked)) && !names(f, path)
		_ = f.Close()
		if !replaced {
			return nil, err
		}
		if !create {
			return nil, os.ErrNotExist
		}
	}
}

// names reports whether f is the file at path.
func names(f *os.File, path string) bool {
	held, err1 := f.Stat()
	named, err2 := os.Stat(path)
	return err1 == nil && err2 == nil && os.SameFile(held, named)
}

// heldBy names the processes holding path open, if it can find them.
func heldBy(path string) string {
	pids, more := lockHolders(path)
	if len(pids) == 0 {
		return ""
	}
	named := make([]string, len(pids))
	for i, pid := range pids {
		named[i] = strconv.Itoa(pid)
	}
	word := "pid "
	if len(pids) > 1 {
		word = "pids "
	}
	if more {
		named = append(named, "...")
	}
	return "; held by " + word + strings.Join(named, ", ")
}

// trashPrefix starts the name a pruned scratch is moved to, which is
// neither a key nor a lock file.
const trashPrefix = ".pruned-"

// prune removes the scratches under root but keep's that no drill used
// for a week and none holds now, with their lock files. Each is moved
// aside under its lock and removed after, so a drill opening it meanwhile
// is not refused.
func prune(root, keep string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), trashPrefix) {
			// Left by a prune that was stopped, or one still removing it.
			_ = removeScratch(filepath.Join(root, e.Name()))
			continue
		}
		key, ok := strings.CutSuffix(e.Name(), ".lock")
		if !ok || key == keep || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(root, e.Name())
		if info, err := os.Stat(path); err != nil || time.Since(info.ModTime()) < unused {
			continue
		}
		lock, err := lockAt(path, false)
		if err != nil {
			continue
		}
		// Still unused once locked: a drill may have used it meanwhile.
		info, err := lock.Stat()
		if err != nil || time.Since(info.ModTime()) < unused {
			_ = lock.Close()
			continue
		}
		trash := filepath.Join(root, trashPrefix+key+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
		moved := os.Rename(filepath.Join(root, key), trash)
		if (moved == nil || errors.Is(moved, fs.ErrNotExist)) && info.Size() == 0 {
			_ = os.Remove(path)
		}
		_ = lock.Close()
		if moved == nil {
			_ = removeScratch(trash)
		}
	}
}

// Seams for tests.
var (
	lockOpened    = func(string) {}
	removeScratch = removeAll
)

// Clear removes the scratch directory and everything in it.
func (s *Scratch) Clear() error { return removeAll(s.Dir) }

// Lock is the open lock file. The lock lasts while any process holds it
// open, so whatever writes in the scratch is handed it.
func (s *Scratch) Lock() *os.File { return s.lock }

// Release gives up the lock; the next drill may then open the scratch.
func (s *Scratch) Release() error { return s.lock.Close() }

func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
