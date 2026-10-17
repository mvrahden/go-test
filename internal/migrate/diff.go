package migrate

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/unidiff"
)

// unifiedDiff renders the edit from before to after as a patch `git apply`
// takes, run in the current directory. A symlink is named by the file it
// points to, which is the one written.
func unifiedDiff(path, shown string, before, after []byte) string {
	name := filepath.ToSlash(shown)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		if cwd, err := os.Getwd(); err == nil {
			if cwd, err := filepath.EvalSymlinks(cwd); err == nil {
				if rel, err := filepath.Rel(cwd, real); err == nil {
					name = filepath.ToSlash(rel)
				}
			}
		}
	}
	return unidiff.Diff("a/"+name, "b/"+name, before, after)
}
