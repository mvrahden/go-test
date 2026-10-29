package cli_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIBinaryCacheTestSuite: what a run's start evicts from the test binary
// cache.
type CLIBinaryCacheTestSuite struct{}

func (s *CLIBinaryCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 30 * time.Second}
}

// staleKey makes a key dir in cache's bin dir that records checkout and
// holds one binary used ago, and returns the binary's path.
func staleKey(t *gotest.T, cache, key, checkout string, ago time.Duration) string {
	dir := filepath.Join(cache, "bin", key)
	gotest.NoError(t, os.MkdirAll(dir, 0o755))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, ".cwd"), []byte(checkout), 0o600))
	bin := filepath.Join(dir, "other.test")
	gotest.NoError(t, os.WriteFile(bin, []byte("binary"), 0o600))
	gotest.NoError(t, os.WriteFile(bin+".lock", nil, 0o600))
	then := time.Now().Add(-ago)
	gotest.NoError(t, os.Chtimes(bin, then, then))
	return bin
}

func (s *CLIBinaryCacheTestSuite) TestWhatARunEvicts(t *gotest.T) {
	cache := t.TempDir()
	gone := staleKey(t, cache, "00000000000000aa", filepath.Join(t.TempDir(), "deleted-worktree"), time.Hour)
	aged := staleKey(t, cache, "00000000000000bb", t.TempDir(), 8*24*time.Hour)
	fresh := staleKey(t, cache, "00000000000000cc", t.TempDir(), time.Hour)
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{protocol.EnvCacheDir + "=" + cache}, "../testdata/passing/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	present := func(path string) bool { _, err := os.Stat(path); return err == nil }

	t.When("a cache key records a checkout that no longer exists", func(w *gotest.T) {
		w.It("a run removes its binaries", func(it *gotest.T) {
			gotest.False(it, present(gone), "%s outlived the run", gone)
		})
	})
	t.When("a cached binary has not been used for 7 days", func(w *gotest.T) {
		w.It("a run removes it and keeps the binaries used since", func(it *gotest.T) {
			gotest.False(it, present(aged), "%s outlived the run", aged)
			gotest.True(it, present(fresh), "%s was evicted", fresh)
		})
	})
}
