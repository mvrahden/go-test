package lifecycle_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// inTreePath is the import path prefix of the testdata packages in the repo.
const inTreePath = "github.com/mvrahden/go-test/tests/lifecycle/"

// stageTestdata copies testdata into a module of its own in a temp dir and
// returns that module's root, so generate writes its harnesses there and
// never into the source tree. Imports of the in-tree path follow the copy.
func stageTestdata(t *gotest.T, repoRoot string) string {
	dir := t.TempDir()
	staged := filepath.Join(dir, "testdata")
	gotest.NoError(t, testkit.StageModule(repoRoot, "testdata", staged))
	gotest.NoError(t, os.Remove(filepath.Join(staged, "go.work")))
	gotest.NoError(t, testkit.WriteModule(repoRoot, dir, "lifecycle"))
	err := filepath.WalkDir(staged, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		src, err := os.ReadFile(path) //nolint:gosec // G304: a file this helper just staged
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(strings.ReplaceAll(string(src), inTreePath, "lifecycle/")), 0o600) //nolint:gosec // G122: a temp dir this helper just made
	})
	gotest.NoError(t, err)
	return dir
}
