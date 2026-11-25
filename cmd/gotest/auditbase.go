package main

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
)

// All of audit's use of git is here: --base exports a commit's tree with git
// archive into a directory of gotest's cache fixed per checkout, which
// leaves the checkout's worktree, index and refs as they were, and records
// it there; the export is kept for the next audit of the same commit. The
// lines the working tree changed against the commit come from git diff.

// gitBase is the commit --base names, in the checkout it was read from.
type gitBase struct {
	ref, commit, short string
	// branch is the branch ref names, "" for a commit.
	branch string
	// top is the checkout's root, prefix the current directory in it.
	top, prefix string
}

// resolveBase reads the commit ref names in the checkout of the current
// directory.
func resolveBase(ref string) (*gitBase, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	top, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--base needs git and a checkout (%s); record the base yourself and give --base-record and --head-record", err)
	}
	commit, err := gitOutput(cwd, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("--base %q names no commit of this checkout; record the base yourself and give --base-record and --head-record", ref)
	}
	prefix, err := gitOutput(cwd, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	short, err := gitOutput(cwd, "rev-parse", "--short", commit)
	if err != nil {
		return nil, err
	}
	return &gitBase{ref: ref, commit: commit, short: short, branch: branchOf(cwd, ref), top: top, prefix: prefix}, nil
}

// branchOf is the local or remote branch ref names, "" for anything else.
func branchOf(dir, ref string) string {
	out, err := gitOutput(dir, "rev-parse", "--symbolic-full-name", "--end-of-options", ref)
	if err != nil {
		return ""
	}
	// rev-parse echoes --end-of-options on a line of its own.
	lines := strings.Split(out, "\n")
	full := lines[len(lines)-1]
	for _, prefix := range []string{"refs/heads/", "refs/remotes/"} {
		if name, ok := strings.CutPrefix(full, prefix); ok {
			return name
		}
	}
	return ""
}

// Files in an export directory beside the tree.
const (
	exportedCommit = "commit"    // the commit the tree holds, once whole
	exportedCopies = "workspace" // the workspace files copied in
)

// recordRef records the tree of the commit, as --record would in the same
// directory of that commit.
func recordRef(g *gitBase, patterns, loadFlags []string) (*gotestaudit.Record, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	held, err := gotestrunner.LockAuditBase(g.top)
	if err != nil {
		return nil, fmt.Errorf("the base's directory: %w", err)
	}
	defer held.Release()
	if within(held.Dir, g.top) {
		return nil, fmt.Errorf("the base's directory %s is inside the checkout; point %s outside it", held.Dir, protocol.EnvCacheDir)
	}
	tree := filepath.Join(held.Dir, "tree")
	if err := exportOnce(ctx, g, held.Dir, tree); err != nil {
		return nil, fmt.Errorf("exporting %s: %w", g.ref, err)
	}

	dir := filepath.Join(tree, filepath.FromSlash(g.prefix))
	present := presentPatterns(dir, patterns)
	clean := false
	prov := gotestaudit.Provenance{Ref: g.ref, Branch: g.branch, Commit: g.short, Dirty: &clean}
	if len(present) == 0 {
		// The commit has none of the directories: every package is new.
		return &gotestaudit.Record{Version: about.ResolvedVersion(), Schema: gotestaudit.Schema, Provenance: prov, Patterns: patterns,
			Notes: []string{gotestaudit.FloorNote}, Packages: []gotestaudit.Package{}, Suppressions: []gotestaudit.Suppression{}, Snapshots: []gotestaudit.Snapshot{}}, nil
	}
	out := filepath.Join(held.Dir, "base.json")
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := append([]string{"audit", "--record", "--output=" + out}, loadFlags...)
	cmd := exec.CommandContext(ctx, exe, append(args, present...)...) //nolint:gosec // G204: this CLI, recording the exported tree
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if ctx.Err() != nil {
			return nil, errors.New("interrupted while recording the base")
		}
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			return nil, fmt.Errorf("recording %s: %w\n%s", g.ref, err, stderr.String())
		}
	}
	rec, err := readRecordFile(out)
	if err != nil {
		return nil, fmt.Errorf("recording %s: %w\n%s", g.ref, err, stderr.String())
	}
	rec.Provenance, rec.Patterns = prov, patterns
	return rec, nil
}

// presentPatterns are the patterns whose directory the tree at dir has: a
// directory the commit lacks holds no package of the base. A pattern that
// names no directory is kept.
func presentPatterns(dir string, patterns []string) []string {
	var out []string
	for _, p := range patterns {
		if !strings.HasPrefix(p, ".") {
			out = append(out, p)
			continue
		}
		root := strings.TrimSuffix(strings.TrimSuffix(p, "..."), "/")
		if root == "" {
			root = "."
		}
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(root))); err == nil && info.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// exportOnce makes tree hold the commit's files and the checkout's
// workspace files, exporting only when it holds another commit or none.
func exportOnce(ctx context.Context, g *gitBase, dir, tree string) error {
	marker := filepath.Join(dir, exportedCommit)
	if got, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(got)) != g.commit { //nolint:gosec // G304: the export's own marker
		if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.RemoveAll(tree); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(dir, exportedCopies))
		if err := exportCommit(ctx, g.top, g.commit, tree); err != nil {
			return err
		}
		if err := os.WriteFile(marker, []byte(g.commit+"\n"), 0o600); err != nil {
			return err
		}
	}
	// The workspace files copied in are the checkout's as they are now.
	if copied, err := os.ReadFile(filepath.Join(dir, exportedCopies)); err == nil { //nolint:gosec // G304: the export's own list
		for _, name := range strings.Fields(string(copied)) {
			_ = os.Remove(filepath.Join(tree, filepath.Base(name)))
		}
	}
	names := keepWorkspace(g.top, tree)
	return os.WriteFile(filepath.Join(dir, exportedCopies), []byte(strings.Join(names, "\n")), 0o600)
}

// within reports whether path is dir or lies under it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// exportCommit writes commit's tracked files into dir: untracked and ignored
// files, generated ones included, are not part of it.
func exportCommit(ctx context.Context, top, commit, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "archive", "--format=tar", commit) //nolint:gosec // G204: git with a verified commit
	cmd.Dir = top
	var stderr strings.Builder
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	extractErr := extractTar(pipe, dir)
	_, _ = io.Copy(io.Discard, pipe)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return extractErr
}

// extractTar writes a tar stream's directories, files and symlinks under dir,
// refusing any name that leaves it.
func extractTar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if !filepath.IsLocal(name) {
			return fmt.Errorf("archive entry %q leaves the tree", h.Name)
		}
		target := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeTarFile(tr, target, h.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		}
	}
}

func writeTarFile(r io.Reader, target string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm|0o600) //nolint:gosec // G304: a file of the exported tree
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil { //nolint:gosec // G110: the checkout's own commit
		_ = f.Close()
		return err
	}
	return f.Close()
}

// keepWorkspace copies the checkout's go.work and go.work.sum into an
// exported tree that has none, so its modules resolve as the checkout's do,
// and returns the names it copied.
func keepWorkspace(top, tree string) []string {
	var copied []string
	for _, name := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(filepath.Join(tree, name)); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(top, name)) //nolint:gosec // G304: the checkout's own workspace file
		if err != nil {
			continue
		}
		if os.WriteFile(filepath.Join(tree, name), data, 0o600) == nil {
			copied = append(copied, name)
		}
	}
	return copied
}

// changedLines are the lines the working tree adds or changes against the
// commit: git diff of the commit and the files on disk, staged or not, and
// every line of a file git neither tracks nor ignores. Paths are relative
// to the checkout's root.
func changedLines(g *gitBase) ([]gotestaudit.FileLines, error) {
	cmd := exec.Command("git", "-c", "core.quotePath=false", "diff", "-U0", "--no-color", "--no-ext-diff", "--no-textconv", //nolint:gosec // G204: git with a verified commit
		"--no-relative", "--find-renames", "--src-prefix=a/", "--dst-prefix=b/", g.commit, "--")
	cmd.Dir = g.top
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	files, err := gotestaudit.ParseDiff(strings.NewReader(string(out)))
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	untracked, err := gitOutput(g.top, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, name := range strings.Split(untracked, "\x00") {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(g.top, filepath.FromSlash(name))) //nolint:gosec // G304: a file of the checkout
		if err != nil {
			continue
		}
		n := strings.Count(string(data), "\n")
		if len(data) > 0 && data[len(data)-1] != '\n' {
			n++
		}
		lines := make([]int, n)
		for i := range lines {
			lines[i] = i + 1
		}
		files = append(files, gotestaudit.FileLines{Path: name, Lines: lines})
	}
	return files, nil
}

// gitOutput runs git in dir and returns its trimmed output; an error names
// what git said.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // G204: git with fixed arguments
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
