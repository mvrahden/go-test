package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BinaryCacheLockCheckTestSuite: a run that takes a binary's lock checks it
// still holds the lock file at that path, and treats the cache as absent
// when an eviction replaced it meanwhile.
// Sequential: it replaces the hook every lock opening runs.
type BinaryCacheLockCheckTestSuite struct{}

func (s *BinaryCacheLockCheckTestSuite) TestALockEvictedWhileWaitingIsNotTheCache(t *gotest.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Windows cannot delete a lock file another handle holds open, so an eviction cannot replace it")
	}
	const pkg = "github.com/mvrahden/go-test/internal/about"
	cache, work := t.TempDir(), t.TempDir()
	cached := filepath.Join(cache, gotestrunner.ExportSanitizePkgName(pkg)+".test")
	overlay := filepath.Join(t.TempDir(), "overlay.json")
	gotest.NoError(t, os.WriteFile(overlay, []byte(`{"Replace":{}}`), 0o600))

	// An eviction holds the lock and has the binary there.
	release := gotest.Must(gotestrunner.ExportLockFile(cached + ".lock"))
	gotest.NoError(t, os.WriteFile(cached, []byte("old"), 0o755))
	opened := make(chan struct{})
	restore := gotestrunner.ExportLockOpened(func(string) { close(opened) })
	type outcome struct {
		res gotestrunner.CompileResult
		err error
	}
	done := make(chan outcome, 1)
	gotest.Go(t, func() {
		res, err := gotestrunner.ExportCompilePackage(context.Background(), pkg, "-overlay="+overlay, nil, work, cache)
		done <- outcome{res, err}
	})
	// The run has opened the lock file and waits on it; the eviction removes
	// the binary and the lock file, then releases.
	<-opened
	restore()
	gotest.NoError(t, os.Remove(cached))
	gotest.NoError(t, os.Remove(cached+".lock"))
	release()
	got := <-done

	t.When("an eviction removes a binary and its lock file while a run waits on that lock", func(w *gotest.T) {
		w.It("links the run's binary into its work dir and leaves none in the cache", func(it *gotest.T) {
			gotest.NoError(it, got.err)
			gotest.Equal(it, filepath.Join(work, filepath.Base(cached)), got.res.BinaryPath)
			_, err := os.Stat(got.res.BinaryPath)
			gotest.NoError(it, err)
			_, err = os.Stat(cached)
			gotest.True(it, os.IsNotExist(err), "a binary was linked into the cache at %s", cached)
		})
	})
}
