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
	t.It("changes with the build flags GOFLAGS carries", func(it *gotest.T) {
		env := func(vars map[string]string) func(string) string {
			return func(name string) string { return vars[name] }
		}
		plain := gotestrunner.ExportEnvKeyFlags(env(nil))
		tagged := gotestrunner.ExportEnvKeyFlags(env(map[string]string{"GOFLAGS": "-tags=probetag"}))
		gotest.Empty(it, plain)
		gotest.NotEqual(it, gotestrunner.ExportBinaryCacheKey("/w", plain), gotestrunner.ExportBinaryCacheKey("/w", tagged))
		gotest.NotEqual(it, tagged, gotestrunner.ExportEnvKeyFlags(env(map[string]string{"GOEXPERIMENT": "-tags=probetag"})))
		gotest.Equal(it, tagged, gotestrunner.ExportEnvKeyFlags(env(map[string]string{"GOFLAGS": "-x -tags=probetag -p=4"})), "trace flags change no binary")
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
	t.It("is another for build tags in GOFLAGS", func(it *gotest.T) {
		root := it.TempDir()
		it.Setenv("GOFLAGS", "")
		plain := gotestrunner.ExportBinaryCacheDir(root, nil)
		it.Setenv("GOFLAGS", "-tags=probetag")
		gotest.NotEqual(it, plain, gotestrunner.ExportBinaryCacheDir(root, nil))
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
		testkit.IsolateTempDir(it)
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

func (s *BinaryCacheKeyTestSuite) TestSums(t *gotest.T) {
	t.It("sums a file to its size and CRC-32C", func(it *gotest.T) {
		path := filepath.Join(it.TempDir(), "a.test")
		gotest.NoError(it, os.WriteFile(path, []byte("abc"), 0o600))
		gotest.Equal(it, gotestrunner.ExportBinarySum{Size: 3, CRC: 0x364b3fb7}, gotest.Must(gotestrunner.ExportSumFile(path)))
	})
	t.It("returns that sum while copying, and an identical copy", func(it *gotest.T) {
		dir := it.TempDir()
		src, dst := filepath.Join(dir, "a.test"), filepath.Join(dir, "b.test")
		gotest.NoError(it, os.WriteFile(src, []byte("abc"), 0o600))
		gotest.Equal(it, gotestrunner.ExportBinarySum{Size: 3, CRC: 0x364b3fb7}, gotest.Must(gotestrunner.ExportCopyFileSum(src, dst)))
		gotest.Equal(it, "abc", string(gotest.Must(os.ReadFile(dst))))
	})
	t.It("reads back the record it wrote", func(it *gotest.T) {
		bin := filepath.Join(it.TempDir(), "a.test")
		want := gotestrunner.ExportBinarySum{Size: 1 << 20, CRC: 0xdeadbeef}
		gotest.NoError(it, gotestrunner.ExportWriteSum(bin, want))
		got, ok := gotestrunner.ExportReadSum(bin)
		gotest.True(it, ok)
		gotest.Equal(it, want, got)
	})
	t.It("reads no record from a malformed or missing file", func(it *gotest.T) {
		bin := filepath.Join(it.TempDir(), "a.test")
		_, ok := gotestrunner.ExportReadSum(bin)
		gotest.False(it, ok)
		gotest.NoError(it, os.WriteFile(bin+".sum", []byte("garbage"), 0o600))
		_, ok = gotestrunner.ExportReadSum(bin)
		gotest.False(it, ok)
	})
}
