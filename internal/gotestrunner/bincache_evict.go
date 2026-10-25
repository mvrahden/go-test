package gotestrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// checkoutFile, in a key dir of the test binary cache, names the working
// directory the key was made for.
const checkoutFile = ".cwd"

// defaultCacheMaxSize caps the test binary cache when GOTEST_CACHE_MAX_SIZE
// is unset: about nine full sets of this repository's binaries.
const defaultCacheMaxSize = 5 << 30

// parseCacheMaxSize reads GOTEST_CACHE_MAX_SIZE: bytes, or a whole number
// with KiB, MiB or GiB; "off" is no cap, returned as 0.
func parseCacheMaxSize(value string) (int64, error) {
	switch value {
	case "":
		return defaultCacheMaxSize, nil
	case "off":
		return 0, nil
	}
	digits, shift := value, 0
	for unit, s := range map[string]int{"KiB": 10, "MiB": 20, "GiB": 30} {
		if d, ok := strings.CutSuffix(value, unit); ok {
			digits, shift = d, s
		}
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n <= 0 || n > (1<<62)>>shift {
		return 0, fmt.Errorf("%s=%s: want a positive number of bytes, optionally with KiB, MiB or GiB, or off for no cap", protocol.EnvCacheMaxSize, value)
	}
	return n << shift, nil
}

// cacheMaxSize is the cap a sweep holds the cache to; a run has refused an
// unreadable value before it sweeps, so one here is the default.
func cacheMaxSize() int64 {
	n, err := parseCacheMaxSize(os.Getenv(protocol.EnvCacheMaxSize))
	if err != nil {
		return defaultCacheMaxSize
	}
	return n
}

// afterScan runs between the sweep's scan and its evictions; tests replace
// it to use the cache meanwhile, as a run would.
var afterScan = func() {}

// keyEmptied runs between removeEmptyKey's check and its removal of the dir;
// tests replace it to create a lock file meanwhile, as a run would.
var keyEmptied = func(dir string) {}

// cachedBinary is one test binary of the cache as the sweep scanned it.
type cachedBinary struct {
	path    string
	size    int64
	modTime time.Time
}

// cacheKeyDir is one key of the cache as the sweep scanned it.
type cacheKeyDir struct {
	dir      string
	gone     bool // its recorded checkout no longer exists
	modTime  time.Time
	binaries []cachedBinary
	locks    []cachedBinary // lock files whose binary is not there
}

// sweepBinaryCache evicts, in order: every binary of a key whose recorded
// checkout no longer exists; every binary unused for cacheMaxAge; then the
// binaries used longest ago until the total is at most maxSize (0: no cap).
// A key recording no checkout is held to the other two rules alone.
func sweepBinaryCache(root string, maxSize int64, now time.Time) {
	keys := scanKeys(filepath.Join(root, "bin"))
	afterScan()
	var kept []cachedBinary
	var total int64
	emptied := map[string]bool{} // key dirs an eviction may have left empty
	for i := range keys {
		k := &keys[i]
		for _, b := range k.binaries {
			if k.gone || now.Sub(b.modTime) > cacheMaxAge {
				if evictBinary(b) {
					emptied[k.dir] = true
				}
				continue
			}
			kept = append(kept, b)
			total += b.size
		}
		if k.gone || now.Sub(k.modTime) > cacheMaxAge {
			for _, l := range k.locks {
				evictLock(l)
			}
		}
	}
	if maxSize > 0 && total > maxSize {
		slices.SortFunc(kept, func(a, b cachedBinary) int { return a.modTime.Compare(b.modTime) })
		for _, b := range kept {
			if total <= maxSize {
				break
			}
			if evictBinary(b) {
				total -= b.size
				emptied[filepath.Dir(b.path)] = true
			}
		}
	}
	for _, k := range keys {
		if k.gone || now.Sub(k.modTime) > cacheMaxAge || emptied[k.dir] {
			removeEmptyKey(k.dir)
		}
	}
}

// scanKeys reads the metadata of every key dir under bin: its binaries with
// the time each was last used (go touches a binary it finds up to date
// instead of relinking it), lock files left without a binary, and whether
// its recorded checkout is gone.
func scanKeys(bin string) []cacheKeyDir {
	entries, err := os.ReadDir(bin)
	if err != nil {
		return nil
	}
	var keys []cacheKeyDir
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		k := cacheKeyDir{dir: filepath.Join(bin, e.Name()), modTime: info.ModTime()}
		if cwd, err := os.ReadFile(filepath.Join(k.dir, checkoutFile)); err == nil {
			k.gone = !exists(string(cwd))
		}
		files, err := os.ReadDir(k.dir)
		if err != nil {
			continue
		}
		names := map[string]bool{}
		for _, f := range files {
			names[f.Name()] = true
		}
		for _, f := range files {
			name := f.Name()
			isBinary := strings.HasSuffix(name, ".test")
			orphanLock := strings.HasSuffix(name, ".test.lock") && !names[strings.TrimSuffix(name, ".lock")]
			if f.IsDir() || !isBinary && !orphanLock {
				continue
			}
			fi, err := f.Info()
			if err != nil {
				continue
			}
			b := cachedBinary{path: filepath.Join(k.dir, name), size: fi.Size(), modTime: fi.ModTime()}
			if isBinary {
				k.binaries = append(k.binaries, b)
			} else {
				k.locks = append(k.locks, b)
			}
		}
		keys = append(keys, k)
	}
	return keys
}

// evictBinary removes b under its lock, taken without waiting: a binary a
// run links or copies is skipped, and so is one used since the scan. It
// reports whether b went.
func evictBinary(b cachedBinary) bool {
	lock := b.path + ".lock"
	unlock, err := tryLockFile(lock)
	if err != nil {
		return false
	}
	info, err := os.Lstat(b.path)
	if err != nil || !info.ModTime().Equal(b.modTime) {
		unlock()
		return false
	}
	if err := os.Remove(b.path); err != nil {
		unlock()
		return false
	}
	releaseEvicted(lock, unlock)
	return true
}

// evictLock removes a lock file left without its binary, under that lock.
func evictLock(l cachedBinary) {
	unlock, err := tryLockFile(l.path)
	if err != nil {
		return
	}
	if exists(strings.TrimSuffix(l.path, ".lock")) {
		unlock()
		return
	}
	releaseEvicted(l.path, unlock)
}

// removeEmptyKey removes a key dir that holds no binary and no lock file:
// its .cwd and any temp file a run that died left writing it first, then
// the dir itself, which fails if a run created a lock meanwhile. That run's
// next use writes .cwd again.
func removeEmptyKey(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".test") || strings.HasSuffix(e.Name(), ".lock") {
			return
		}
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), checkoutFile+".tmp-") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	_ = os.Remove(filepath.Join(dir, checkoutFile))
	keyEmptied(dir)
	_ = os.Remove(dir)
}
