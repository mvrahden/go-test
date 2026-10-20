package testkit

import (
	"context"
	"os"
	"path/filepath"
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

// cachedCLI puts a CLI built from repoRoot's sources into dir, reusing the
// build published in cache for the same sources. Each caller runs a copy of
// its own, so a later build for other sources never replaces it.
func cachedCLI(ctx context.Context, repoRoot, dir, cache string) (string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	cache, err = filepath.Abs(cache)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cache, 0o750); err != nil {
		return "", err
	}
	s := store{dir: cache, pid: os.Getpid(), rules: defaultSweep}
	s.sweep()
	return reuse(ctx, s, root, dir)
}
