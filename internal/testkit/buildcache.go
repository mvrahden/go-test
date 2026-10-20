package testkit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// ChildCacheDir is the gotest cache of the runs whose markers go to markerDir.
// GOTEST_TEST_CACHE_DIR names one root that runs share instead; reuse is sound
// only while the sources stay put, so its owner resets it when they change.
func ChildCacheDir(markerDir string) string {
	if root := os.Getenv("GOTEST_TEST_CACHE_DIR"); root != "" {
		return root
	}
	return filepath.Join(markerDir, "cache")
}

// cachedCLI always asks Go to validate a stable output, under a lock, then
// copies it into the caller's private directory. Neither a source change nor
// a concurrent build can leave a caller executing another mutant's binary.
// The mutex covers goroutines an fcntl lock would not exclude.
func cachedCLI(ctx context.Context, repoRoot, dir, cache string) (string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(root)))
	cache, err = filepath.Abs(cache)
	if err != nil {
		return "", err
	}
	cachedDir := filepath.Join(cache, key)
	if err := os.MkdirAll(cachedDir, 0o750); err != nil {
		return "", err
	}
	mu, _ := cacheLocks.LoadOrStore(cachedDir, new(sync.Mutex))
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	lock, err := openLock(cachedDir + ".lock")
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := lockCacheFD(lock, false); err != nil {
		return "", err
	}
	defer unlockFD(lock)
	binary, err := buildCachedCLI(ctx, root, cachedDir)
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

// cacheLocks holds a *sync.Mutex per absolute cached build dir.
var cacheLocks sync.Map

// lockCacheFD and buildCachedCLI are cachedCLI's lock and build; tests swap them.
var (
	lockCacheFD    = lockFD
	buildCachedCLI = buildCLI
)
