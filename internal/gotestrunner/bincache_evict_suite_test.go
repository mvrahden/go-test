package gotestrunner_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BinaryCacheEvictionTestSuite covers what the run-start sweep evicts from
// the test binary cache.
// Sequential: each method points GOTEST_CACHE_DIR and the temp dir at its own.
type BinaryCacheEvictionTestSuite struct{}

// cacheKey makes a key dir under root's bin dir holding the named binaries
// and their lock files, recording cwd as its checkout unless cwd is empty.
func cacheKey(t *gotest.T, root, key, cwd string, binaries ...string) string {
	dir := filepath.Join(root, "bin", key)
	gotest.NoError(t, os.MkdirAll(dir, 0o755))
	if cwd != "" {
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, ".cwd"), []byte(cwd), 0o600))
	}
	for _, b := range binaries {
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, b), []byte("binary"), 0o755))
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, b+".lock"), nil, 0o644))
	}
	return dir
}

// isolated points the cache and the temp dir at fresh dirs and returns the
// cache root.
func isolated(t *gotest.T) string {
	testkit.IsolateTempDir(t)
	root := t.TempDir()
	t.Setenv(protocol.EnvCacheDir, root)
	return root
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *BinaryCacheEvictionTestSuite) TestAGoneCheckoutLosesItsBinaries(t *gotest.T) {
	t.When("a key records a checkout that no longer exists", func(w *gotest.T) {
		w.It("loses every binary, and the key dir with them", func(it *gotest.T) {
			root := isolated(it)
			gone := filepath.Join(it.TempDir(), "deleted-worktree")
			dir := cacheKey(it, root, "aaaaaaaaaaaaaaaa", gone, "a.test", "b.test")

			gotestrunner.CleanStaleOverlays()

			gotest.False(it, exists(filepath.Join(dir, "a.test")))
			gotest.False(it, exists(filepath.Join(dir, "b.test")))
			gotest.False(it, exists(dir), "the empty key dir %s is left", dir)
		})
	})
	t.When("a key records a checkout that exists", func(w *gotest.T) {
		w.It("keeps its binaries", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "bbbbbbbbbbbbbbbb", it.TempDir(), "a.test")

			gotestrunner.CleanStaleOverlays()

			gotest.True(it, exists(filepath.Join(dir, "a.test")))
		})
	})
}

func (s *BinaryCacheEvictionTestSuite) TestAKeyWithoutACheckout(t *gotest.T) {
	t.When("a key written before keys recorded their checkout is swept", func(w *gotest.T) {
		w.It("keeps its fresh binaries", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "cccccccccccccccc", "", "a.test")

			gotestrunner.CleanStaleOverlays()

			gotest.True(it, exists(filepath.Join(dir, "a.test")))
		})
	})
	t.When("a run uses a key", func(w *gotest.T) {
		w.It("records the run's checkout in it", func(it *gotest.T) {
			root := isolated(it)
			dir := gotestrunner.ExportBinaryCacheDir(root, nil)
			cwd, err := os.Getwd()
			gotest.NoError(it, err)
			got, err := os.ReadFile(filepath.Join(dir, ".cwd"))
			gotest.NoError(it, err)
			gotest.Equal(it, cwd, string(got))
		})
	})
}

// aged sets path's modification time to ago before now.
func aged(t *gotest.T, path string, ago time.Duration) {
	then := time.Now().Add(-ago)
	gotest.NoError(t, os.Chtimes(path, then, then))
}

func (s *BinaryCacheEvictionTestSuite) TestTheCapEvictsTheOldestFirst(t *gotest.T) {
	t.When("the cache holds more than its cap", func(w *gotest.T) {
		w.It("evicts the binaries used longest ago until the total is at or under the cap", func(it *gotest.T) {
			root := isolated(it)
			cwd := it.TempDir()
			a := cacheKey(it, root, "dddddddddddddddd", cwd, "old.test", "new.test")
			b := cacheKey(it, root, "eeeeeeeeeeeeeeee", cwd, "older.test", "newer.test")
			// Six bytes each ("binary"); used 4, 3, 2 and 1 hours ago.
			aged(it, filepath.Join(b, "older.test"), 4*time.Hour)
			aged(it, filepath.Join(a, "old.test"), 3*time.Hour)
			aged(it, filepath.Join(b, "newer.test"), 2*time.Hour)
			aged(it, filepath.Join(a, "new.test"), time.Hour)
			it.Setenv(protocol.EnvCacheMaxSize, "12")

			gotestrunner.CleanStaleOverlays()

			gotest.False(it, exists(filepath.Join(b, "older.test")))
			gotest.False(it, exists(filepath.Join(a, "old.test")))
			gotest.True(it, exists(filepath.Join(b, "newer.test")))
			gotest.True(it, exists(filepath.Join(a, "new.test")))
		})
	})
	t.When("a binary has not been used for 7 days", func(w *gotest.T) {
		w.It("evicts it though the cache is under its cap", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "ffffffffffffffff", it.TempDir(), "stale.test", "fresh.test")
			aged(it, filepath.Join(dir, "stale.test"), 8*24*time.Hour)

			gotestrunner.CleanStaleOverlays()

			gotest.False(it, exists(filepath.Join(dir, "stale.test")))
			gotest.True(it, exists(filepath.Join(dir, "fresh.test")))
		})
	})
	t.When("the cap is off", func(w *gotest.T) {
		w.It("keeps every binary used within 7 days", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "abababababababab", it.TempDir(), "a.test", "b.test")
			it.Setenv(protocol.EnvCacheMaxSize, "off")

			gotestrunner.CleanStaleOverlays()

			gotest.True(it, exists(filepath.Join(dir, "a.test")))
			gotest.True(it, exists(filepath.Join(dir, "b.test")))
		})
	})
}

func (s *BinaryCacheEvictionTestSuite) TestTheCapsSpelling(t *gotest.T) {
	type row struct {
		Desc  string
		value string
		want  int64
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "unset is the 5 GiB default", value: "", want: 5 << 30},
		{Desc: "bytes", value: "1024", want: 1024},
		{Desc: "KiB", value: "3KiB", want: 3 << 10},
		{Desc: "MiB", value: "200MiB", want: 200 << 20},
		{Desc: "GiB", value: "2GiB", want: 2 << 30},
		{Desc: "off is no cap", value: "off", want: 0},
	}) {
		got, err := gotestrunner.ExportParseCacheMaxSize(r.value)
		gotest.NoError(t, err)
		gotest.Equal(t, r.want, got)
	}
	for t, bad := range gotest.Each(t, []struct{ Desc string }{{Desc: "5GB"}, {Desc: "-1"}, {Desc: "0"}, {Desc: "lots"}, {Desc: "1.5GiB"}}) {
		_, err := gotestrunner.ExportParseCacheMaxSize(bad.Desc)
		gotest.ErrorContains(t, err, protocol.EnvCacheMaxSize)
	}
}

func (s *BinaryCacheEvictionTestSuite) TestARunRefusesAnUnreadableCap(t *gotest.T) {
	isolated(t)
	t.Setenv(protocol.EnvCacheMaxSize, "5GB")
	_, _, err := gotestrunner.GenerateOverlay(nil, nil, false, false, false)
	t.It("fails before it generates anything, naming the variable", func(it *gotest.T) {
		gotest.ErrorContains(it, err, protocol.EnvCacheMaxSize+"=5GB")
	})
}

func (s *BinaryCacheEvictionTestSuite) TestEvictionNeverTakesABinaryInUse(t *gotest.T) {
	t.When("the binary to evict first is locked by a run", func(w *gotest.T) {
		w.It("skips it and evicts the next one instead", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "1212121212121212", it.TempDir(), "held.test", "next.test", "newest.test")
			aged(it, filepath.Join(dir, "held.test"), 3*time.Hour)
			aged(it, filepath.Join(dir, "next.test"), 2*time.Hour)
			aged(it, filepath.Join(dir, "newest.test"), time.Hour)
			release := gotest.Must(gotestrunner.ExportLockFile(filepath.Join(dir, "held.test.lock")))
			it.Setenv(protocol.EnvCacheMaxSize, "12")

			gotestrunner.CleanStaleOverlays()
			release()

			gotest.True(it, exists(filepath.Join(dir, "held.test")))
			gotest.False(it, exists(filepath.Join(dir, "next.test")))
			gotest.True(it, exists(filepath.Join(dir, "newest.test")))
		})
	})
	t.When("a run uses a binary between the sweep's scan and its eviction", func(w *gotest.T) {
		w.It("keeps it", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "3434343434343434", it.TempDir(), "a.test")
			bin := filepath.Join(dir, "a.test")
			aged(it, bin, 8*24*time.Hour)
			restore := gotestrunner.ExportAfterScan(func() { aged(it, bin, 0) })

			gotestrunner.CleanStaleOverlays()
			restore()

			gotest.True(it, exists(bin))
		})
	})
	t.When("a run creates a lock file in a key dir the sweep is removing", func(w *gotest.T) {
		w.It("keeps the key dir and the lock file", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "5656565656565656", filepath.Join(it.TempDir(), "gone"), "a.test")
			fresh := filepath.Join(dir, "b.test.lock")
			restore := gotestrunner.ExportKeyEmptied(func(string) {
				gotest.NoError(it, os.WriteFile(fresh, nil, 0o644))
			})

			gotestrunner.CleanStaleOverlays()
			restore()

			gotest.False(it, exists(filepath.Join(dir, "a.test")))
			gotest.True(it, exists(fresh))
		})
	})
	t.When("a binary is evicted", func(w *gotest.T) {
		w.It("takes its lock file with it", func(it *gotest.T) {
			root := isolated(it)
			dir := cacheKey(it, root, "7878787878787878", it.TempDir(), "a.test", "b.test")
			aged(it, filepath.Join(dir, "a.test"), 8*24*time.Hour)

			gotestrunner.CleanStaleOverlays()

			gotest.False(it, exists(filepath.Join(dir, "a.test")))
			gotest.False(it, exists(filepath.Join(dir, "a.test.lock")))
			gotest.True(it, exists(filepath.Join(dir, "b.test.lock")))
		})
	})
}
