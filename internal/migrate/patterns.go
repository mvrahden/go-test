package migrate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// patternDirs lists the directories pattern names, by one rule for every
// spelling. A path (., ./x, ../x, /x, and .\x or C:\x on Windows) is read
// from disk, a file as its directory; an import path is mapped onto the
// directory of the main module that holds it. A trailing /... (or \...)
// adds every directory below with Go files, skipping what ./... skips.
// A directory counts when it holds Go files under any build constraint,
// since migrate checks the tagged ones too; a pattern that matches none
// fails.
func patternDirs(pattern string) ([]string, error) {
	root, recursive := cutWildcard(pattern, filepath.Separator)
	if !isPathPattern(pattern, filepath.Separator) {
		dir, err := importPathDir(root)
		if err != nil {
			return nil, fmt.Errorf("pattern %s: %w", pattern, err)
		}
		root = dir
	}
	if root == "" {
		root = "."
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("pattern %s: %w", pattern, err)
	}
	if !info.IsDir() {
		if recursive {
			return nil, fmt.Errorf("pattern %s: %s is not a directory", pattern, root)
		}
		return []string{filepath.Dir(root)}, nil
	}
	if !recursive {
		if !hasGoFiles(root) {
			return nil, fmt.Errorf("pattern %s: no Go files in %s", pattern, root)
		}
		return []string{root}, nil
	}
	var dirs []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != root {
			// What ./... leaves out: testdata, vendor, hidden and _ dirs,
			// and nested modules.
			name := d.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		if hasGoFiles(path) {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err == nil && len(dirs) == 0 {
		err = fmt.Errorf("pattern %s: matched no packages", pattern)
	}
	return dirs, err
}

// cutWildcard strips a trailing /... (or sep...) off pattern.
func cutWildcard(pattern string, sep rune) (root string, recursive bool) {
	if pattern == "..." {
		return "", true
	}
	for _, suffix := range []string{"/...", string(sep) + "..."} {
		if root, ok := strings.CutSuffix(pattern, suffix); ok {
			return root, true
		}
	}
	return pattern, false
}

// isPathPattern reports whether pattern is a path, not an import path:
// relative with . or .., or absolute. sep is the OS path separator.
func isPathPattern(pattern string, sep rune) bool {
	if pattern == "." || pattern == ".." {
		return true
	}
	for _, prefix := range []string{"./", "../", "." + string(sep), ".." + string(sep)} {
		if strings.HasPrefix(pattern, prefix) {
			return true
		}
	}
	if sep == '\\' {
		// C:\x, C:/x and \\server\share
		return len(pattern) >= 3 && pattern[1] == ':' && (pattern[2] == '\\' || pattern[2] == '/') || strings.HasPrefix(pattern, `\\`)
	}
	return strings.HasPrefix(pattern, "/")
}

// importPathDir is the directory of the main module holding importPath.
// migrate edits files in place, so a dependency's is no target.
func importPathDir(importPath string) (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}\t{{.Dir}}")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	best, bestDir := "", ""
	for line := range strings.Lines(string(out)) {
		mod, dir, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || dir == "" {
			continue
		}
		if (importPath == mod || strings.HasPrefix(importPath, mod+"/")) && len(mod) > len(best) {
			best, bestDir = mod, dir
		}
	}
	if best == "" {
		return "", fmt.Errorf("%s is not in the main module; migrate reads directories and import paths of the module it runs in", importPath)
	}
	dir := filepath.Join(bestDir, filepath.FromSlash(strings.TrimPrefix(importPath, best)))
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			dir = rel
		}
	}
	return dir, nil
}

// hasGoFiles reports whether dir holds a .go file.
func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}
