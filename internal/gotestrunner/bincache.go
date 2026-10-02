package gotestrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"time"
)

// A run links its test binaries into a cache keyed by what decides the
// link's output: the working directory (two checkouts of one module share
// import paths), the build flags, the platform and the Go version. The -o
// path is then stable across runs, and go skips the link when the binary's
// build ID still matches. Each run copies the binary into its own work dir:
// a concurrent run relinking the same key never replaces a file another run
// is executing, and the budget file beside a binary stays the run's own. The
// link runs under a per-binary lock so two runs cannot write one output at
// once.

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
	return dir
}

// copyFile writes src's bytes to dst as an executable, replacing dst. No
// process is forked while dst is open: a child forked then would hold the
// write handle until it execs, and the kernel refuses to execute a file
// anyone holds open for writing ("text file busy").
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	_ = os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
