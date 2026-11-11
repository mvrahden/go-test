package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// readProvenance is the checkout dir is in: its branch, its commit and
// whether it has uncommitted changes. Outside a checkout, or without git,
// it is the zero value.
func readProvenance(dir string) gotestspec.Provenance {
	branch, err := git(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return gotestspec.Provenance{}
	}
	commit, err := git(dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return gotestspec.Provenance{}
	}
	status, err := git(dir, "status", "--porcelain")
	if err != nil {
		return gotestspec.Provenance{}
	}
	return gotestspec.Provenance{Branch: branch, Commit: commit, Dirty: status != ""}
}

// git runs git in dir and returns its trimmed output.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // G204: git with fixed arguments
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// modulePath is the path of the module enclosing dir, "" outside one.
func modulePath(dir string) string {
	for root := dir; ; {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return gotestspec.ReadModulePath(root)
		}
		parent := filepath.Dir(root)
		if parent == root {
			return ""
		}
		root = parent
	}
}

// Whether a report is of a run made here or of a saved stream, which says
// nothing of the checkout it was made in.
const (
	liveRun     = true
	savedStream = false
)

// reportOptions are the module and, for a live run, the checkout a report
// names.
func reportOptions(live bool) []gotestspec.RenderOption {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	opts := []gotestspec.RenderOption{gotestspec.WithModule(modulePath(cwd))}
	if live {
		opts = append(opts, gotestspec.WithProvenance(readProvenance(cwd)))
	}
	return opts
}

// terminalOptions are what the terminal spec adds to a view: the module, the
// checkout and the width it wraps at. A spec written to a file is not wrapped,
// whatever stdout is.
func terminalOptions(w io.Writer, live bool) []gotestspec.RenderOption {
	width := terminalWidth()
	if f, ok := w.(*os.File); ok && f != os.Stdout {
		width = 0
	}
	return append(reportOptions(live), gotestspec.WithWidth(width))
}
