package gotestrunner_test

import (
	"os"
	"path/filepath"

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
