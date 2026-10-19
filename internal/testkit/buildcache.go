package testkit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ChildCacheDir separates reusable build artifacts from a run's marker files.
// The drill supplies a private worker root; other callers keep their existing
// per-run cache. Go still validates every build against the current sources.
func ChildCacheDir(markerDir string) string {
	if root := os.Getenv("GOTEST_TEST_CACHE_DIR"); root != "" {
		return root
	}
	return filepath.Join(markerDir, "cache")
}

// cachedCLI always asks Go to validate a stable output, under a lock, then
// copies it into the caller's private directory. Neither a source change nor
// a concurrent build can leave a caller executing another mutant's binary.
func cachedCLI(ctx context.Context, repoRoot, dir, cache string) (string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(root)))
	cachedDir := filepath.Join(cache, key)
	if err := os.MkdirAll(cachedDir, 0o750); err != nil {
		return "", err
	}
	lock, err := openLock(cachedDir + ".lock")
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := lockFD(lock, false); err != nil {
		return "", err
	}
	defer unlockFD(lock)
	binary, err := buildCLI(ctx, root, cachedDir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", err
	}
	private := filepath.Join(dir, cliName())
	// A concurrent fork must not inherit a write handle to an executable
	// another goroutine is about to start (the same rule as runner.copyFile).
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	if err := os.WriteFile(private, data, 0o700); err != nil { //nolint:gosec // G306: the caller must execute its private CLI
		return "", err
	}
	return private, nil
}
