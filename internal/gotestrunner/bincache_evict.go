package gotestrunner

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// checkoutFile, in a key dir of the test binary cache, names the working
// directory the key was made for.
const checkoutFile = ".cwd"

// cachedBinary is one test binary of the cache as the sweep scanned it.
type cachedBinary struct {
	path    string
	modTime time.Time
}

// sweepBinaryCache evicts the binaries of every key whose recorded checkout
// no longer exists. A key that records none is left to the other rules.
func sweepBinaryCache(root string) {
	bin := filepath.Join(root, "bin")
	keys, err := os.ReadDir(bin)
	if err != nil {
		return
	}
	for _, k := range keys {
		if !k.IsDir() {
			continue
		}
		dir := filepath.Join(bin, k.Name())
		cwd, err := os.ReadFile(filepath.Join(dir, checkoutFile))
		if err != nil || exists(string(cwd)) {
			continue
		}
		for _, b := range scanBinaries(dir) {
			evictBinary(b)
		}
		removeEmptyKey(dir)
	}
}

// scanBinaries lists dir's test binaries with the time each was last used:
// go touches a binary it finds up to date instead of relinking it.
func scanBinaries(dir string) []cachedBinary {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []cachedBinary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".test") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, cachedBinary{path: filepath.Join(dir, e.Name()), modTime: info.ModTime()})
	}
	return out
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

// removeEmptyKey removes a key dir that holds no binary and no lock file:
// its .cwd first, then the dir itself, which fails if a run created a lock
// meanwhile. That run's next use writes .cwd again.
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
	_ = os.Remove(filepath.Join(dir, checkoutFile))
	_ = os.Remove(dir)
}
