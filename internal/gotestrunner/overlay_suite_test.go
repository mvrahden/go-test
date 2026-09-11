package gotestrunner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OverlayTestSuite covers overlay generation and the content-addressed write cache.
// Sequential: Setenv (the cache directory).
type OverlayTestSuite struct{}

func (s *OverlayTestSuite) TestOverlayManagement(t *gotest.T) {
	t.When("writing overlay", func(w *gotest.T) {
		w.It("creates correct overlay entries for PTest and PXTest", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n"), PXTest: []byte("package a_test\n")},
				{AbsPath: "/fake/pkg/b", PTest: []byte("package b\n")},
			}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			err = json.Unmarshal(data, &ov)
			gotest.NoError(it, err)

			wantKeys := map[string]bool{
				filepath.Join("/fake/pkg/a", about.PSuite):  true,
				filepath.Join("/fake/pkg/a", about.PXSuite): true,
				filepath.Join("/fake/pkg/b", about.PSuite):  true,
			}
			gotest.Len(it, wantKeys, len(ov.Replace))
			for virtual, real := range ov.Replace {
				gotest.True(it, wantKeys[virtual], "unexpected overlay key: %s", virtual)
				_, err := os.Stat(real)
				gotest.NoError(it, err)
			}

			bPXSuite := filepath.Join("/fake/pkg/b", about.PXSuite)
			_, ok := ov.Replace[bPXSuite]
			gotest.False(it, ok, "pkg/b should not have PXSuite mapping (empty PXTest)")
		})

		w.It("produces unique overlay directories when called twice", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n")},
			}

			dir1, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(dir1)

			dir2, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(dir2)

			gotest.NotEqual(it, dir1, dir2)
		})

		w.It("contains a .pid file with the current PID", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n")},
			}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, ".pid"))
			gotest.NoError(it, err)

			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			gotest.NoError(it, err)
			gotest.Equal(it, os.Getpid(), pid)
		})

		w.It("creates an empty overlay for nil results", func(it *gotest.T) {
			tmpDir, err := gotestrunner.WriteOverlay(nil)
			gotest.NoError(it, err)
			defer os.RemoveAll(tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			err = json.Unmarshal(data, &ov)
			gotest.NoError(it, err)
			gotest.Empty(it, ov.Replace)
		})
	})

	t.When("cleaning stale overlays", func(w *gotest.T) {
		w.It("removes overlay with dead PID", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			// Write a PID that doesn't exist (use a very high PID)
			_ = os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0600)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected stale overlay to be removed")
		})

		w.It("keeps overlay with live PID", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			defer os.RemoveAll(dir)

			// Write our own PID -- guaranteed alive
			_ = os.WriteFile(filepath.Join(dir, ".pid"), []byte(strconv.Itoa(os.Getpid())), 0600)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.False(it, os.IsNotExist(err), "expected live overlay to be kept")
		})

		w.It("removes overlay with no PID file", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected overlay without PID file to be removed")
		})

		w.It("ignores non-overlay directories", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "unrelated-")
			gotest.NoError(it, err)
			defer os.RemoveAll(dir)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.False(it, os.IsNotExist(err), "expected non-overlay dir to be untouched")
		})
	})
}

func (s *OverlayTestSuite) TestOverlayCache(t *gotest.T) {
	t.When("computing content hash", func(w *gotest.T) {
		w.It("produces deterministic hash for same content", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("package a\n")},
				{AbsPath: "/pkg/b", PTest: []byte("package b\n"), PXTest: []byte("package b_test\n")},
			}
			h1 := gotestrunner.ExportOverlayContentHash(results)
			h2 := gotestrunner.ExportOverlayContentHash(results)
			gotest.Equal(it, h1, h2)
			gotest.Len(it, h1, 64)
		})

		w.It("is order-independent (sorted by AbsPath)", func(it *gotest.T) {
			r1 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("aaa")},
				{AbsPath: "/pkg/b", PTest: []byte("bbb")},
			}
			r2 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/b", PTest: []byte("bbb")},
				{AbsPath: "/pkg/a", PTest: []byte("aaa")},
			}
			gotest.Equal(it, gotestrunner.ExportOverlayContentHash(r1), gotestrunner.ExportOverlayContentHash(r2))
		})

		w.It("changes when content changes", func(it *gotest.T) {
			r1 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("version1")},
			}
			r2 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("version2")},
			}
			gotest.NotEqual(it, gotestrunner.ExportOverlayContentHash(r1), gotestrunner.ExportOverlayContentHash(r2))
		})

		w.It("changes when AbsPath changes", func(it *gotest.T) {
			r1 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("same")},
			}
			r2 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/b", PTest: []byte("same")},
			}
			gotest.NotEqual(it, gotestrunner.ExportOverlayContentHash(r1), gotestrunner.ExportOverlayContentHash(r2))
		})

		w.It("returns empty-stable hash for nil results", func(it *gotest.T) {
			h1 := gotestrunner.ExportOverlayContentHash(nil)
			h2 := gotestrunner.ExportOverlayContentHash(gotestgen.GenerateResults{})
			gotest.Equal(it, h1, h2)
			gotest.Len(it, h1, 64)
		})
	})

	t.When("cache root resolution", func(w *gotest.T) {
		w.It("respects GOTEST_CACHE_DIR env var", func(it *gotest.T) {
			dir := it.TempDir()
			it.Setenv(protocol.EnvCacheDir, dir)

			root, err := gotestrunner.ExportCacheRoot()
			gotest.NoError(it, err)
			gotest.Equal(it, dir, root)
		})

		w.It("falls back to UserCacheDir/gotest when env is unset", func(it *gotest.T) {
			it.Setenv(protocol.EnvCacheDir, "")

			root, err := gotestrunner.ExportCacheRoot()
			gotest.NoError(it, err)
			gotest.True(it, strings.HasSuffix(root, "gotest"))
			gotest.NotEmpty(it, root)
		})
	})

	t.When("writing to cache", func(w *gotest.T) {
		w.It("creates valid overlay in cache directory", func(it *gotest.T) {
			cacheDir := it.TempDir()
			it.Setenv(protocol.EnvCacheDir, cacheDir)

			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/m", PTest: []byte("package m\n"), PXTest: []byte("package m_test\n")},
			}

			hash := gotestrunner.ExportOverlayContentHash(results)
			expectedDir := filepath.Join(cacheDir, "overlays", hash)

			// Ensure it doesn't exist yet.
			_, err := os.Stat(expectedDir)
			gotest.True(it, os.IsNotExist(err))

			// Write via the exported WriteOverlay — this uses tmpdir path.
			// For cache path, we test the internal via GenerateOverlay with noCache=false.
			// Since GenerateOverlay needs loaded packages, test the cache write separately.
			// We'll use writeOverlayCached indirectly by calling the exported function.
			dir, err := gotestrunner.ExportWriteOverlayCached(results, false)
			gotest.NoError(it, err)

			gotest.Equal(it, expectedDir, dir)

			// Verify overlay.json exists and is valid.
			data, err := os.ReadFile(filepath.Join(dir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			gotest.NoError(it, json.Unmarshal(data, &ov))
			gotest.Len(it, ov.Replace, 2)
		})

		w.It("returns same directory on repeated calls (cache hit)", func(it *gotest.T) {
			cacheDir := it.TempDir()
			it.Setenv(protocol.EnvCacheDir, cacheDir)

			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/r", PTest: []byte("package r\n")},
			}

			dir1, err := gotestrunner.ExportWriteOverlayCached(results, false)
			gotest.NoError(it, err)

			dir2, err := gotestrunner.ExportWriteOverlayCached(results, false)
			gotest.NoError(it, err)

			gotest.Equal(it, dir1, dir2)
		})

		w.It("falls back to tmpdir when noCache is true", func(it *gotest.T) {
			cacheDir := it.TempDir()
			it.Setenv(protocol.EnvCacheDir, cacheDir)

			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/nc", PTest: []byte("package nc\n")},
			}

			dir, err := gotestrunner.ExportWriteOverlayCached(results, true)
			gotest.NoError(it, err)
			defer os.RemoveAll(dir)

			// Should be in tmpdir, not in cache.
			gotest.False(it, strings.HasPrefix(dir, cacheDir))
		})
	})

	t.When("cleaning old cache entries", func(w *gotest.T) {
		w.It("removes entries older than 7 days", func(it *gotest.T) {
			cacheDir := it.TempDir()
			it.Setenv(protocol.EnvCacheDir, cacheDir)

			overlaysDir := filepath.Join(cacheDir, "overlays")
			_ = os.MkdirAll(overlaysDir, 0755)

			// Create an old entry.
			oldDir := filepath.Join(overlaysDir, "old-hash-entry")
			_ = os.MkdirAll(oldDir, 0755)
			_ = os.WriteFile(filepath.Join(oldDir, "overlay.json"), []byte("{}"), 0600)
			oldTime := time.Now().Add(-8 * 24 * time.Hour)
			_ = os.Chtimes(oldDir, oldTime, oldTime)

			// Create a fresh entry.
			freshDir := filepath.Join(overlaysDir, "fresh-hash-entry")
			_ = os.MkdirAll(freshDir, 0755)
			_ = os.WriteFile(filepath.Join(freshDir, "overlay.json"), []byte("{}"), 0600)

			gotestrunner.CleanStaleOverlays()

			_, err := os.Stat(oldDir)
			gotest.True(it, os.IsNotExist(err), "expected old cache entry to be removed")

			_, err = os.Stat(freshDir)
			gotest.False(it, os.IsNotExist(err), "expected fresh cache entry to be kept")
		})
	})
}
