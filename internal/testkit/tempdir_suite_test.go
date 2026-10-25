package testkit_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TempDirTestSuite covers pointing the temp dir at one of a test's own.
type TempDirTestSuite struct{}

func (s *TempDirTestSuite) TestIsolateTempDir(t *gotest.T) {
	was := os.TempDir()
	t.When("a test isolates its temp dir", func(w *gotest.T) {
		testkit.IsolateTempDir(w)
		dir := os.Getenv("TMPDIR")
		w.It("points TMPDIR, TMP and TEMP at one fresh dir of its own", func(it *gotest.T) {
			gotest.NotEqual(it, was, dir)
			gotest.Equal(it, dir, os.Getenv("TMP"))
			gotest.Equal(it, dir, os.Getenv("TEMP"))
			entries, err := os.ReadDir(dir)
			gotest.NoError(it, err)
			gotest.Empty(it, entries)
		})
		w.It("makes it the dir os.TempDir names", func(it *gotest.T) {
			gotest.Equal(it, filepath.Clean(dir), filepath.Clean(os.TempDir()))
		})
	})
}
