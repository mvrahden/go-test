package gotestrunner_test

import (
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
	const pkg = "m/a"
	cache, stage := t.TempDir(), filepath.Join(t.TempDir(), "stage")
	cached := filepath.Join(cache, gotestrunner.ExportSanitizePkgName(pkg)+".test")
	record := func() {
		gotest.NoError(t, gotestrunner.ExportWriteSum(cached, gotest.Must(gotestrunner.ExportSumFile(cached))))
	}

	// An eviction holds the lock and has the binary there.
	release := gotest.Must(gotestrunner.ExportLockFile(cached + ".lock"))
	gotest.NoError(t, os.WriteFile(cached, []byte("old"), 0o600))
	record()
	opened := make(chan struct{})
	restore := gotestrunner.ExportLockOpened(func(string) { close(opened) })
	type outcome struct {
		staged []gotestrunner.StagedBinary
		err    error
	}
	done := make(chan outcome, 1)
	gotest.Go(t, func() {
		staged, err := gotestrunner.ExportStageGroup([]string{pkg}, stage, cache)
		done <- outcome{staged, err}
	})
	// The run has opened the lock file and waits on it; the eviction removes
	// the binary and the lock file, then releases. A run that did not wait
	// linked the binary again meanwhile, under a lock file of its own.
	<-opened
	restore()
	gotest.NoError(t, os.Remove(cached))
	gotest.NoError(t, os.Remove(cached+".lock"))
	gotest.NoError(t, os.WriteFile(cached, []byte("new"), 0o600))
	record()
	release()
	got := <-done

	t.When("an eviction removes a binary and its lock file while a run waits on that lock", func(w *gotest.T) {
		w.It("places nothing from the cache, so go links the run's binary itself", func(it *gotest.T) {
			gotest.NoError(it, got.err)
			gotest.False(it, gotestrunner.ExportPlaced(&got.staged[0]))
			_, err := os.Stat(got.staged[0].Path)
			gotest.True(it, os.IsNotExist(err), "a binary was placed at %s", got.staged[0].Path)
		})
	})
}
