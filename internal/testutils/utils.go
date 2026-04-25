package testutils

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var (
	timestampRegex = regexp.MustCompile(`(\d+\.\d+s|\(cached\))`)
)

func CompareTestOutputWithGolden(t *testing.T, tmp string, actual *bytes.Buffer, testFS embed.FS, goldenName string) {
	expected := openGolden(t, testFS, goldenName)
	t.Helper()
	actualStr := actual.String()
	actualStr = strings.ReplaceAll(actualStr, tmp, "<REPLACED>")
	actualStr = timestampRegex.ReplaceAllString(actualStr, "<TIMESTAMP>")
	gotest.Equal(t, expected.String(), actualStr)
}

func openGolden(t *testing.T, testFS embed.FS, name string) *bytes.Buffer {
	buf, err := testFS.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed reading golden: %s", err)
	}
	return bytes.NewBuffer(buf)
}

func CopyModuleUnderTestToTmp(t *testing.T, tmp string, modPath string, skipByPrefix ...string) {
	t.Helper()
	err := copyFS(tmp, os.DirFS(modPath), skipByPrefix...)
	if err != nil {
		t.Fatalf("failed copying: %s", err)
	}
}

// ActivateTests replaces ".go.test" suffix of file names to ".go".
func ActivateTests(t *testing.T, tmp string) {
	t.Helper()
	tmpFS := os.DirFS(tmp)
	err := fs.WalkDir(tmpFS, ".", func(path string, d fs.DirEntry, err error) error {
		if strings.HasSuffix(d.Name(), ".go.test") {
			return os.Rename(
				filepath.Join(tmp, path),
				filepath.Join(tmp, filepath.Dir(path), strings.TrimSuffix(d.Name(), ".test")))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed renaming file: %s", err)
	}
}

// AssertFilesInTmp activates test files, which are intended to be executed only
// in a Temp-dir and are excluded from execution via `go test`.
func AssertFilesInTmp(t *testing.T, path string, expectedFiles ...string) {
	t.Helper()
	_, notFound := determineNotFoundFiles(t, path, expectedFiles...)
	if len(notFound) > 0 {
		t.Fatalf("could not find expected files: %+v", notFound)
	}
}

func AssertFilesNotInTmp(t *testing.T, path string, unexpectedFiles ...string) {
	t.Helper()
	found, _ := determineNotFoundFiles(t, path, unexpectedFiles...)
	if len(found) > 0 {
		t.Fatalf("found unexpected files: %+v", found)
	}
}

func determineNotFoundFiles(t *testing.T, path string, searchFiles ...string) (found, notFound []string) {
	t.Helper()
	for _, v := range searchFiles {
		_, err := os.Stat(filepath.Join(path, v))
		notExists := errors.Is(err, os.ErrNotExist)
		if err != nil && !notExists {
			t.Fatalf("failed reading pwd: %s", err)
		}
		if notExists {
			notFound = append(notFound, v)
		} else {
			found = append(found, v)
		}
	}
	return found, notFound
}

func HackGoWork(t *testing.T, tmp string) {
	t.Helper()
	out, err := exec.
		Command("bash", "-c", fmt.Sprintf("cd %s && go mod tidy", tmp)).
		CombinedOutput()
	if err != nil {
		t.Fatalf("cmd failed: %s: output: %s", err, string(out))
	}
	out, err = exec.
		Command("bash", "-c", fmt.Sprintf("cd %s && go work init %s && go work use %s/examples", tmp, tmp, tmp)).
		CombinedOutput()
	if err != nil {
		t.Fatalf("cmd failed: %s: output: %s", err, string(out))
	}
}

// copied from: https://github.com/golang/go/issues/62484#issue-1884498794
func copyFS(dir string, fsys fs.FS, skipRootLevels ...string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		// assert path matches
		if slices.ContainsFunc(skipRootLevels, func(v string) bool {
			return strings.HasPrefix(path, v)
		}) {
			return nil // skip
		}

		targ := filepath.Join(dir, filepath.FromSlash(path))

		if d.IsDir() {
			if err := os.MkdirAll(targ, 0777); err != nil {
				return err
			}
			return nil
		}
		if slices.Contains(skipRootLevels, path) {
			return nil
		}
		r, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()
		info, err := r.Stat()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(targ, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666|info.Mode()&0777)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			w.Close()
			return fmt.Errorf("copying %s: %v", path, err)
		}
		if err := w.Close(); err != nil {
			return err
		}
		return nil
	})
}
