package gotestrunner_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// Sequential: TestEviction points GOTEST_CACHE_DIR at a temp dir.
type BinaryCacheKeyTestSuite struct{}

func (s *BinaryCacheKeyTestSuite) TestKey(t *gotest.T) {
	t.It("is stable for one working directory and flag set, in any flag order", func(it *gotest.T) {
		a := gotestrunner.ExportBinaryCacheKey("/w", []string{"-race", "-tags=x"})
		gotest.Equal(it, a, gotestrunner.ExportBinaryCacheKey("/w", []string{"-tags=x", "-race"}))
		gotest.Len(it, a, 16)
	})
	t.It("changes with the working directory, since two checkouts share import paths", func(it *gotest.T) {
		gotest.NotEqual(it, gotestrunner.ExportBinaryCacheKey("/w", nil), gotestrunner.ExportBinaryCacheKey("/v", nil))
	})
	t.It("changes with the build flags", func(it *gotest.T) {
		gotest.NotEqual(it, gotestrunner.ExportBinaryCacheKey("/w", nil), gotestrunner.ExportBinaryCacheKey("/w", []string{"-race"}))
	})
}

func (s *BinaryCacheKeyTestSuite) TestDir(t *gotest.T) {
	t.It("is empty when the cache is off", func(it *gotest.T) {
		gotest.Empty(it, gotestrunner.ExportBinaryCacheDir("", nil))
	})
	t.It("lives in a keyed bin directory of the root and exists", func(it *gotest.T) {
		root := it.TempDir()
		dir := gotest.Must(filepath.Rel(root, gotestrunner.ExportBinaryCacheDir(root, nil)))
		gotest.Regexp(it, `^bin[\\/][0-9a-f]{16}$`, dir)
		_, err := os.Stat(filepath.Join(root, dir))
		gotest.NoError(it, err)
	})
}

func (s *BinaryCacheKeyTestSuite) TestLock(t *gotest.T) {
	t.It("holds a second taker until the first releases", func(it *gotest.T) {
		path := filepath.Join(it.TempDir(), "x.lock")
		unlock := gotest.Must(gotestrunner.ExportLockFile(path))
		second := make(chan struct{})
		gotest.Go(it, func() {
			u := gotest.Must(gotestrunner.ExportLockFile(path))
			close(second)
			u()
		})
		gotest.Consistently(it, 200*time.Millisecond, 20*time.Millisecond, func(poll *gotest.R) {
			select {
			case <-second:
				gotest.Fail(poll, "second lock taken while the first is held")
			default:
			}
		})
		unlock()
		gotest.Eventually(it, 2*time.Second, 20*time.Millisecond, func(poll *gotest.R) {
			select {
			case <-second:
			default:
				gotest.Fail(poll, "second lock not taken after release")
			}
		})
	})
}

func (s *BinaryCacheKeyTestSuite) TestEviction(t *gotest.T) {
	t.It("removes bin entries older than the cache age and keeps fresh ones", func(it *gotest.T) {
		root := it.TempDir()
		it.Setenv(protocol.EnvCacheDir, root)
		old := filepath.Join(root, "bin", "0000000000000000")
		fresh := filepath.Join(root, "bin", "1111111111111111")
		gotest.NoError(it, os.MkdirAll(old, 0o755))
		gotest.NoError(it, os.MkdirAll(fresh, 0o755))
		stale := time.Now().Add(-8 * 24 * time.Hour)
		gotest.NoError(it, os.Chtimes(old, stale, stale))
		gotestrunner.CleanStaleOverlays()
		_, err := os.Stat(old)
		gotest.Error(it, err)
		_, err = os.Stat(fresh)
		gotest.NoError(it, err)
	})
}
