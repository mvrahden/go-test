package cli_test

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIBinaryCacheTestSuite: the test binary cache evicts under load without
// taking a binary from a run that uses it.
type CLIBinaryCacheTestSuite struct{}

func (s *CLIBinaryCacheTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Measured on linux: three rounds of 8 runs ~5s. Not yet measured on
	// Windows CI, whose process starts and file writes are far slower; 2m
	// leaves room, as for the concurrent overlay runs.
	return gotest.SuiteConfig{Parallel: true, Timeout: 2 * time.Minute}
}

func (s *CLIBinaryCacheTestSuite) TestRunsUnderATinyCap(t *gotest.T) {
	const rounds, runs = 3, 8
	env := []string{protocol.EnvCacheDir + "=" + t.TempDir(), protocol.EnvCacheMaxSize + "=1"}
	var results []testkit.Run
	var errs []error
	for range rounds {
		round := make([]testkit.Run, runs)
		roundErrs := make([]error, runs)
		var wg sync.WaitGroup
		for i := range runs {
			markers := t.TempDir()
			wg.Go(func() {
				round[i], roundErrs[i] = testkit.RunPackageWithEnv(t.Context(), markers, env, "../testdata/passing/")
			})
		}
		wg.Wait()
		results, errs = append(results, round...), append(errs, roundErrs...)
	}

	t.When("8 runs at once, in 3 rounds, share a cache capped at one byte", func(w *gotest.T) {
		w.It("passes every run, each suite running and reporting", func(it *gotest.T) {
			for i, run := range results {
				gotest.NoError(it, errs[i])
				gotest.Equal(it, 0, run.Exit, "run %d stdout: %s\nstderr: %s", i, run.Stdout, run.Stderr)
				gotest.Equal(it, "pass", run.Verdict("TestPassingTestSuite"), "run %d stdout: %s", i, run.Stdout)
			}
		})
	})
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

func (s *CLIBinaryCacheTestSuite) TestAnUnreadableCap(t *gotest.T) {
	cache := t.TempDir()
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{protocol.EnvCacheDir + "=" + cache, protocol.EnvCacheMaxSize + "=5GB"}, "../testdata/passing/")
	gotest.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(cache, "bin"))

	t.When("GOTEST_CACHE_MAX_SIZE is not a size", func(w *gotest.T) {
		w.It("exits 2 naming the variable, before any build", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit, "stderr: %s", run.Stderr)
			gotest.Contains(it, run.Stderr+run.Stdout, "GOTEST_CACHE_MAX_SIZE=5GB")
			gotest.True(it, os.IsNotExist(statErr), "a build ran: %s exists", filepath.Join(cache, "bin"))
		})
	})
}

func (s *CLIBinaryCacheTestSuite) TestTheCapGoesOldestFirst(t *gotest.T) {
	cache := t.TempDir()
	checkout := t.TempDir()
	oldest := staleKey(t, cache, "00000000000000d1", checkout, 3*time.Hour)
	older := staleKey(t, cache, "00000000000000d2", checkout, 2*time.Hour)
	newer := staleKey(t, cache, "00000000000000d3", checkout, time.Hour)
	// Six bytes each: a cap of 12 keeps two, swept before this run links.
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{protocol.EnvCacheDir + "=" + cache, protocol.EnvCacheMaxSize + "=12"}, "../testdata/passing/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	present := func(path string) bool { _, err := os.Stat(path); return err == nil }

	t.When("the cache holds more than GOTEST_CACHE_MAX_SIZE", func(w *gotest.T) {
		w.It("a run removes the binaries used longest ago until it is at or under the cap", func(it *gotest.T) {
			gotest.False(it, present(oldest), "%s outlived the run", oldest)
			gotest.True(it, present(older), "%s was evicted", older)
			gotest.True(it, present(newer), "%s was evicted", newer)
		})
	})
}
