package main_test

import (
	"errors"
	"os"
	"time"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LintTempTestSuite covers how a stopped lint removes its temporary dir,
// into which the compiles of the go command it stopped may still write.
// Sequential: each method swaps the removal the package uses.
type LintTempTestSuite struct{}

func (s *LintTempTestSuite) TestTheTempDirIsRemovedOnceNothingWritesThere(t *gotest.T) {
	t.When("removing the dir fails while a stopped compile still writes into it", func(w *gotest.T) {
		w.It("tries again until the dir is gone", func(it *gotest.T) {
			dir := it.TempDir()
			calls := 0
			main.ExportRemoveLintTemp(dir, 5*time.Second, func(path string) error {
				calls++
				if calls <= 3 {
					return errors.New("directory not empty")
				}
				return os.RemoveAll(path)
			})
			gotest.Equal(it, 4, calls)
			_, err := os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "%s is left", dir)
		})
	})
	t.When("removing the dir succeeds at once", func(w *gotest.T) {
		w.It("waits for nothing", func(it *gotest.T) {
			calls := 0
			began := time.Now()
			// No disk work inside the timing: a real removal can take 50ms
			// on Windows, as long as the one sleep this guards against.
			main.ExportRemoveLintTemp(it.TempDir(), 5*time.Second, func(string) error {
				calls++
				return nil
			})
			gotest.Equal(it, 1, calls)
			gotest.Less(it, time.Since(began), 50*time.Millisecond)
		})
	})
	t.When("removing the dir keeps failing", func(w *gotest.T) {
		w.It("gives up at the bound", func(it *gotest.T) {
			began := time.Now()
			main.ExportRemoveLintTemp(it.TempDir(), 300*time.Millisecond, func(string) error { return errors.New("busy") })
			gotest.Less(it, time.Since(began), 2*time.Second)
		})
	})
}
