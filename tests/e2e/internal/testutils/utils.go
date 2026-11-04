// Package testutils stages a copy of this repository as a consumer would
// hold it.
package testutils

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// excluded are the root-level paths a copy leaves out: VCS state, the
// checkout's own workspace, this suite and the editor extension.
var excluded = []string{".git", "go.work", "tests/e2e", "vscode-gotest", "node_modules", ".worktrees"}

// CopyModule copies the module at modPath into dir, leaving out excluded.
func CopyModule(t *gotest.T, dir, modPath string) {
	gotest.NoError(t, copyFS(dir, os.DirFS(modPath), excluded...))
}

// UseExamplesWorkspace tidies the copy and gives it a go.work holding the
// copy and its examples module.
func UseExamplesWorkspace(t *gotest.T, dir string) {
	for _, args := range [][]string{
		{"mod", "tidy"},
		{"work", "init", dir},
		{"work", "use", filepath.Join(dir, "examples")},
	} {
		cmd := exec.CommandContext(t.Context(), "go", args...) //nolint:gosec // G204: go tool with controlled arguments
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		gotest.NoError(t, err, "go %s: %s", strings.Join(args, " "), out)
	}
}

// copyFS copies fsys into dir, skipping paths that start with a skip prefix.
func copyFS(dir string, fsys fs.FS, skip ...string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if slices.ContainsFunc(skip, func(v string) bool { return strings.HasPrefix(path, v) }) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		targ := filepath.Join(dir, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(targ, 0o777)
		}
		if !d.Type().IsRegular() {
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
		w, err := os.OpenFile(targ, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o666|info.Mode()&0o777)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			w.Close()
			return fmt.Errorf("copying %s: %w", path, err)
		}
		return w.Close()
	})
}
