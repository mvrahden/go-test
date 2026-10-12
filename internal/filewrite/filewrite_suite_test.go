package filewrite_test

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/mvrahden/go-test/internal/filewrite"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type FilewriteTestSuite struct{}

func (s *FilewriteTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// file writes "old" to name in dir with perm and returns its change to "new".
func file(t *gotest.T, dir, name string, perm os.FileMode) filewrite.Change {
	path := filepath.Join(dir, name)
	gotest.NoError(t, os.WriteFile(path, []byte("old"), perm))
	return filewrite.Change{File: path, Before: []byte("old"), After: []byte("new")}
}

func read(t *gotest.T, path string) string {
	src, err := os.ReadFile(path)
	gotest.NoError(t, err)
	return string(src)
}

func names(t *gotest.T, dir string) []string {
	entries, err := os.ReadDir(dir)
	gotest.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// write stages and commits changes.
func write(changes ...filewrite.Change) (int, error) {
	staged, err := filewrite.Stage(changes)
	if err != nil {
		return 0, err
	}
	written, err := staged.Commit()
	return len(written), err
}

// posix skips where permission bits do not bind, as for root.
func posix(t *gotest.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skipf("POSIX permissions as a non-root user")
	}
}

func (s *FilewriteTestSuite) TestWrite(t *gotest.T) {
	t.When("every file can be written", func(w *gotest.T) {
		dir := w.TempDir()
		a, b := file(w, dir, "a.go", 0o640), file(w, dir, "b.go", 0o600)
		n, err := write(a, b)

		w.It("writes them all and leaves no temporary file", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, 2, n)
			gotest.Equal(it, "new", read(it, a.File))
			gotest.Equal(it, "new", read(it, b.File))
			gotest.Equal(it, []string{"a.go", "b.go"}, names(it, dir))
		})
		w.It("keeps each file's mode", func(it *gotest.T) {
			if runtime.GOOS == "windows" {
				it.Skipf("permission bits are POSIX")
			}
			info, err := os.Stat(a.File)
			gotest.NoError(it, err)
			gotest.Equal(it, os.FileMode(0o640), info.Mode().Perm())
		})
	})

	t.When("a file has a second hard link", func(w *gotest.T) {
		dir := w.TempDir()
		a := file(w, dir, "a.go", 0o600)
		other := filepath.Join(dir, "other.go")
		gotest.NoError(w, os.Link(a.File, other))
		_, err := write(a)

		w.It("writes the file both names share", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, "new", read(it, a.File))
			gotest.Equal(it, "new", read(it, other))
		})
	})

	t.When("a file has an extended attribute", func(w *gotest.T) {
		dir := w.TempDir()
		a := file(w, dir, "a.go", 0o600)
		if err := setXattr(a.File, "user.gotest", "kept"); err != nil {
			w.Skipf("no extended attributes here: %v", err)
		}
		_, err := write(a)
		value, xerr := getXattr(a.File, "user.gotest")

		w.It("keeps it", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, "new", read(it, a.File))
			gotest.NoError(it, xerr)
			gotest.Equal(it, "kept", value)
		})
	})

	t.When("a file is a symlink", func(w *gotest.T) {
		if runtime.GOOS == "windows" {
			w.Skipf("symlinks need privileges on Windows")
		}
		dir := w.TempDir()
		real := file(w, w.TempDir(), "real.go", 0o600)
		link := filepath.Join(dir, "link.go")
		gotest.NoError(w, os.Symlink(real.File, link))
		_, err := write(filewrite.Change{File: link, Before: real.Before, After: real.After})
		info, lerr := os.Lstat(link)

		w.It("writes the file it names and keeps the link", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, "new", read(it, real.File))
			gotest.NoError(it, lerr)
			gotest.NotZero(it, info.Mode()&os.ModeSymlink)
		})
	})

	t.When("a writable file sits in a directory that is not", func(w *gotest.T) {
		posix(w)
		dir := w.TempDir()
		a := file(w, dir, "a.go", 0o600)
		gotest.NoError(w, os.Chmod(dir, 0o500))
		n, err := write(a)
		gotest.NoError(w, os.Chmod(dir, 0o700))

		w.It("writes it in place", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, 1, n)
			gotest.Equal(it, "new", read(it, a.File))
		})
	})

	t.When("one file cannot be written", func(w *gotest.T) {
		posix(w)
		dir := w.TempDir()
		a, b := file(w, dir, "a.go", 0o600), file(w, dir, "b.go", 0o400)
		n, err := write(a, b)

		w.It("fails with the reason and writes none, leaving no temporary file", func(it *gotest.T) {
			gotest.ErrorIs(it, err, os.ErrPermission)
			gotest.Zero(it, n)
			gotest.Equal(it, "old", read(it, a.File))
			gotest.Equal(it, "old", read(it, b.File))
			gotest.Equal(it, []string{"a.go", "b.go"}, names(it, dir))
		})
	})

	t.When("a file changed since it was read", func(w *gotest.T) {
		dir := w.TempDir()
		a, b := file(w, dir, "a.go", 0o600), file(w, dir, "b.go", 0o600)
		b.Before = []byte("older")
		_, err := write(a, b)

		w.It("fails naming it and writes none", func(it *gotest.T) {
			gotest.ErrorContains(it, err, b.File+" changed since it was read")
			gotest.Equal(it, "old", read(it, a.File))
			gotest.Equal(it, []string{"a.go", "b.go"}, names(it, dir))
		})
	})
}

func (s *FilewriteTestSuite) TestCommit(t *gotest.T) {
	t.When("a write fails partway", func(w *gotest.T) {
		dir := w.TempDir()
		a, b := file(w, dir, "a.go", 0o600), file(w, dir, "b.go", 0o600)
		staged, err := filewrite.Stage([]filewrite.Change{a, b})
		gotest.NoError(w, err)
		// b's staged replacement goes, so its rename fails.
		tmp, err := filepath.Glob(filepath.Join(dir, ".b.go.gotest-*"))
		gotest.NoError(w, err)
		gotest.Len(w, tmp, 1)
		gotest.NoError(w, os.Remove(tmp[0]))
		written, err := staged.Commit()
		n := len(written)

		w.It("stops there and reports how many it wrote", func(it *gotest.T) {
			gotest.ErrorIs(it, err, os.ErrNotExist)
			gotest.Equal(it, 1, n)
			gotest.Equal(it, "new", read(it, a.File))
			gotest.Equal(it, "old", read(it, b.File))
			gotest.Equal(it, []string{"a.go", "b.go"}, names(it, dir))
		})
	})

	t.When("staged changes are discarded", func(w *gotest.T) {
		dir := w.TempDir()
		a := file(w, dir, "a.go", 0o600)
		staged, err := filewrite.Stage([]filewrite.Change{a})
		gotest.NoError(w, err)
		staged.Discard()

		w.It("writes nothing and leaves no temporary file", func(it *gotest.T) {
			gotest.Equal(it, "old", read(it, a.File))
			gotest.Equal(it, []string{"a.go"}, names(it, dir))
		})
	})
}

// inPlace stages a's change to longer contents, written in place: a has a
// second hard link.
func inPlace(t *gotest.T) (filewrite.Change, *filewrite.Staged) {
	dir := t.TempDir()
	a := file(t, dir, "a.go", 0o600)
	a.After = []byte("new and longer")
	gotest.NoError(t, os.Link(a.File, filepath.Join(dir, "other.go")))
	staged, err := filewrite.Stage([]filewrite.Change{a})
	gotest.NoError(t, err)
	return a, staged
}

// failingWrite writes 3 bytes of the first call, then fails it and the
// next fails more calls.
func failingWrite(fails int) func(f *os.File, b []byte, off int64) (int, error) {
	calls := 0
	return func(f *os.File, b []byte, off int64) (int, error) {
		calls++
		if calls > fails {
			return f.WriteAt(b, off)
		}
		n, _ := f.WriteAt(b[:min(3, len(b))], off)
		return n, syscall.ENOSPC
	}
}

func (s *FilewriteTestSuite) TestWriteFailsMidway(t *gotest.T) {
	t.When("a file written in place cannot be written whole", func(w *gotest.T) {
		a, staged := inPlace(w)
		filewrite.SetWriteAt(staged, failingWrite(1))
		written, err := staged.Commit()
		n := len(written)

		w.It("puts its old contents back and says so", func(it *gotest.T) {
			gotest.Equal(it, 0, n)
			gotest.ErrorContains(it, err, "a.go is left as it was")
			gotest.Equal(it, "old", read(it, a.File))
		})
	})

	t.When("its old contents cannot be put back either", func(w *gotest.T) {
		a, staged := inPlace(w)
		filewrite.SetWriteAt(staged, failingWrite(2))
		_, err := staged.Commit()

		w.It("says it may be partly written", func(it *gotest.T) {
			gotest.ErrorContains(it, err, "a.go may be partly written")
			gotest.ErrorIs(it, err, syscall.ENOSPC)
			gotest.Equal(it, a.File, gotest.ErrorAs[*filewrite.PartlyWrittenError](it, err).File)
			gotest.NotEqual(it, "new and longer", read(it, a.File))
		})
	})
}

func (s *FilewriteTestSuite) TestRenameRefused(t *gotest.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.go", 0o600)
	staged, err := filewrite.Stage([]filewrite.Change{a})
	gotest.NoError(t, err)
	// As a file bind-mounted from another filesystem refuses it.
	filewrite.SetRename(staged, func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	})
	written, err := staged.Commit()
	n := len(written)

	t.It("writes the file in place and leaves no temporary file", func(it *gotest.T) {
		gotest.NoError(it, err)
		gotest.Equal(it, 1, n)
		gotest.Equal(it, "new", read(it, a.File))
		gotest.Equal(it, []string{"a.go"}, names(it, dir))
	})
}

func (s *FilewriteTestSuite) TestOneFileTwice(t *gotest.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("symlinks need privileges on Windows")
	}
	// a and a link to it, changed through both names.
	twice := func(t *gotest.T, after string) (filewrite.Change, filewrite.Change) {
		dir := t.TempDir()
		a := file(t, dir, "a.go", 0o600)
		link := filepath.Join(dir, "link.go")
		gotest.NoError(t, os.Symlink(a.File, link))
		return a, filewrite.Change{File: link, Before: a.Before, After: []byte(after)}
	}

	t.When("both changes agree", func(w *gotest.T) {
		a, viaLink := twice(w, "new")
		n, err := write(a, viaLink)

		w.It("writes the file, reporting both names written", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, 2, n)
			gotest.Equal(it, "new", read(it, a.File))
		})
	})

	t.When("they differ", func(w *gotest.T) {
		a, viaLink := twice(w, "other")
		b := file(w, w.TempDir(), "b.go", 0o600)
		_, err := write(b, a, viaLink)

		w.It("refuses them and writes nothing", func(it *gotest.T) {
			gotest.ErrorContains(it, err, "are one file, changed two ways")
			gotest.Equal(it, "old", read(it, a.File))
			gotest.Equal(it, "old", read(it, b.File))
		})
	})
}
