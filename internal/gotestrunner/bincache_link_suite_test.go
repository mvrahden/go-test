package gotestrunner_test

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BinaryLinkTestSuite: a cached binary reaches a run by a hard link, by a
// copy where the link is refused.
// Sequential: it replaces the link function every placement calls.
type BinaryLinkTestSuite struct{}

func (s *BinaryLinkTestSuite) TestLinkOrCopy(t *gotest.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "binary")
	gotest.NoError(t, os.WriteFile(src, []byte("binary"), 0o600))
	dst, dst2, dst3, dst4 := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c"), filepath.Join(dir, "d")

	t.It("links on the same filesystem", func(it *gotest.T) {
		if runtime.GOOS == "windows" {
			it.Skipf("Windows copies: a linked binary in use cannot be replaced or evicted")
		}
		linked, err := gotestrunner.ExportLinkOrCopy(src, dst)
		gotest.NoError(it, err)
		gotest.True(it, linked)
		a, _ := os.Stat(src)
		b, _ := os.Stat(dst)
		gotest.True(it, os.SameFile(a, b))
	})
	t.It("copies when the link is refused", func(it *gotest.T) {
		restore := gotestrunner.ExportSetHardLink(func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EXDEV} })
		defer restore()
		linked, err := gotestrunner.ExportLinkOrCopy(src, dst2)
		gotest.NoError(it, err)
		gotest.False(it, linked)
		a, _ := os.Stat(src)
		b, _ := os.Stat(dst2)
		gotest.False(it, os.SameFile(a, b))
		got, _ := os.ReadFile(dst2)
		gotest.Equal(it, "binary", string(got))
	})
	t.It("replaces what the destination held", func(it *gotest.T) {
		gotest.NoError(it, os.WriteFile(dst3, []byte("old"), 0o600))
		_, err := gotestrunner.ExportLinkOrCopy(src, dst3)
		gotest.NoError(it, err)
		got, _ := os.ReadFile(dst3)
		gotest.Equal(it, "binary", string(got))
	})
	t.It("copies on Windows", func(it *gotest.T) {
		if runtime.GOOS != "windows" {
			it.Skipf("the link is taken here; the refused-link case covers the copy")
		}
		linked, err := gotestrunner.ExportLinkOrCopy(src, dst4)
		gotest.NoError(it, err)
		gotest.False(it, linked)
	})
}
