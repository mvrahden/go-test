package gotestrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"
)

// A run links its test binaries into a cache keyed by what decides the
// link's output: the working directory (two checkouts of one module share
// import paths), the build flags, the platform and the Go version. The -o
// path is then stable across runs, and go skips the link when the binary's
// build ID still matches. Each run copies the binary into its own work dir:
// a concurrent run relinking the same key never replaces a file another run
// is executing. The link runs under a per-binary lock so two runs cannot
// write one output at once.

func binaryCacheKey(cwd string, buildFlags []string) string {
	flags := slices.Clone(buildFlags)
	slices.Sort(flags)
	h := sha256.New()
	for _, part := range append([]string{cwd, runtime.GOOS, runtime.GOARCH, runtime.Version()}, flags...) {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// binaryCacheDir returns the directory this run's binaries link into, or ""
// when the cache is off (an empty root) or cannot be used.
func binaryCacheDir(root string, buildFlags []string) string {
	if root == "" {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := filepath.Join(root, "bin", binaryCacheKey(cwd, buildFlags))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	now := time.Now()
	_ = os.Chtimes(dir, now, now)
	recordCheckout(dir, cwd)
	return dir
}

// recordCheckout writes cwd into the key dir's .cwd unless it is there
// already, so the sweep can tell a key whose checkout is gone. It goes
// through a rename: a sweep never reads half a path.
func recordCheckout(dir, cwd string) {
	if got, err := os.ReadFile(filepath.Join(dir, checkoutFile)); err == nil && string(got) == cwd {
		return
	}
	tmp, err := os.CreateTemp(dir, checkoutFile+".tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.WriteString(cwd)
	if cerr := tmp.Close(); werr != nil || cerr != nil || os.Rename(tmp.Name(), filepath.Join(dir, checkoutFile)) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// errLockReplaced is a lock taken on a file no longer at its path: an
// eviction removed it while this run waited, and the cache entry it guarded
// is gone. The run treats the cache as absent.
var errLockReplaced = errors.New("lock file replaced while waiting")

// stillAt reports whether path still names f.
func stillAt(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	now, err := os.Stat(path)
	return err == nil && os.SameFile(held, now)
}

// lockOpened runs once a lock file is open and before it is locked; tests
// replace it to act while a run waits on a lock.
var lockOpened = func(path string) {}

// hardLink makes a hard link; tests replace it.
var hardLink = os.Link

// linkOrCopy makes dst name src's bytes as an executable, replacing dst:
// a hard link, or a copy on Windows, where a linked binary in use can be
// neither replaced nor evicted, and wherever the link is refused (another
// filesystem, protected hard links, a link count at its limit).
func linkOrCopy(src, dst string) (linked bool, err error) {
	if runtime.GOOS == "windows" {
		return false, copyFile(src, dst)
	}
	_ = os.Remove(dst)
	if hardLink(src, dst) == nil {
		return true, nil
	}
	return false, copyFile(src, dst)
}

// copyFile writes src's bytes to dst as an executable, replacing dst. No
// process is forked while dst is open: a child forked then would hold the
// write handle until it execs, and the kernel refuses to execute a file
// anyone holds open for writing ("text file busy").
func copyFile(src, dst string) error {
	_, err := copyFileSum(src, dst)
	return err
}
