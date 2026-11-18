package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIBinaryCacheTestSuite: what a run's start evicts from the test binary
// cache, and what it reuses. Each method has a cache of its own.
type CLIBinaryCacheTestSuite struct{}

func (s *CLIBinaryCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
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

// cachedLink matches a link action in the build trace GOFLAGS=-x prints.
var cachedLink = regexp.MustCompile(`(?m)(^|[/\\])link(\.exe)?"? `)

// cachedRun runs the passing package with cache as the binary cache.
func cachedRun(t *gotest.T, cache string, env ...string) testkit.Run {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), append([]string{protocol.EnvCacheDir + "=" + cache}, env...), "../testdata/passing/")
	gotest.NoError(t, err)
	return run
}

// cachedBinary is the one test binary a run left in cache.
func cachedBinary(t *gotest.T, cache string) string {
	bins, err := filepath.Glob(filepath.Join(cache, "bin", "*", "*.test"))
	gotest.NoError(t, err)
	gotest.Len(t, bins, 1)
	return bins[0]
}

func (s *CLIBinaryCacheTestSuite) TestACachedBinaryDamagedSinceItWasLinked(t *gotest.T) {
	t.When("a cached test binary is truncated, or zero-filled at its end, after a run linked it", func(w *gotest.T) {
		w.It("the next run links it again, says so on stderr, and passes", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct {
				Desc   string
				damage func(f *os.File, size int64) error
			}{
				{"truncated to half", func(f *os.File, n int64) error { return f.Truncate(n / 2) }},
				{"its last 4 KiB zero-filled", func(f *os.File, n int64) error { _, err := f.WriteAt(make([]byte, 4096), n-4096); return err }},
			}) {
				cache := sub.TempDir()
				first := cachedRun(sub, cache)
				gotest.Equal(sub, 0, first.Exit, "stderr: %s", first.Stderr)
				bin := cachedBinary(sub, cache)
				info, err := os.Stat(bin)
				gotest.NoError(sub, err)
				f, err := os.OpenFile(bin, os.O_WRONLY, 0)
				gotest.NoError(sub, err)
				gotest.NoError(sub, errors.Join(row.damage(f, info.Size()), f.Close()))
				run := cachedRun(sub, cache)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				gotest.Contains(sub, run.Stderr, "gotest: the cached test binary of")
			}
		})
	})
}

func (s *CLIBinaryCacheTestSuite) TestACachedBinaryNoRunRecorded(t *gotest.T) {
	t.When("a cached test binary has no record beside it", func(w *gotest.T) {
		cache := w.TempDir()
		first := cachedRun(w, cache)
		gotest.Equal(w, 0, first.Exit, "stderr: %s", first.Stderr)
		bin := cachedBinary(w, cache)
		gotest.NoError(w, os.Remove(bin+".sum"))
		run := cachedRun(w, cache, "GOFLAGS=-x")
		_, recorded := os.Stat(bin + ".sum")
		w.It("the next run links it again without a word and records it", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.NotContains(it, run.Stderr, "gotest: the cached test binary of")
			gotest.Len(it, cachedLink.FindAllString(run.Stderr, -1), 1, run.Stderr)
			gotest.NoError(it, recorded)
		})
	})
}

func (s *CLIBinaryCacheTestSuite) TestClearingTheCache(t *gotest.T) {
	t.When("gotest clean --cache runs", func(w *gotest.T) {
		cache := w.TempDir()
		first := cachedRun(w, cache)
		gotest.Equal(w, 0, first.Exit, "stderr: %s", first.Stderr)
		env := []string{protocol.EnvCacheDir + "=" + cache}
		clean, err := testkit.RunCLIWithEnv(w.Context(), ".", env, "clean", "--cache")
		gotest.NoError(w, err)
		left, err := filepath.Glob(filepath.Join(cache, "bin", "*", "*.test"))
		gotest.NoError(w, err)
		refused, err := testkit.RunCLIWithEnv(w.Context(), ".", env, "clean", "--cache", "../testdata/passing/")
		gotest.NoError(w, err)
		w.It("removes every cached binary no run is using and says how many", func(it *gotest.T) {
			gotest.Equal(it, 0, clean.Exit, "stderr: %s", clean.Stderr)
			gotest.Empty(it, left)
			gotest.Contains(it, clean.Stderr, "removed 1 cached test binaries")
		})
		w.It("refuses a package beside the flag with exit 2", func(it *gotest.T) {
			gotest.Equal(it, 2, refused.Exit, "stderr: %s", refused.Stderr)
		})
	})
}
