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

	"github.com/mvrahden/go-test/internal/gotestaudit"
)

// All of audit's use of git is here: --base exports a commit's tree with git
// archive into a temporary directory, which leaves the checkout's worktree,
// index and refs as they were, records it there and removes it.

// recordRef records the tree of the commit ref names, as --record would in
// the same directory of that commit.
func recordRef(ref string, patterns, loadFlags []string) (*gotestaudit.Record, error) {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir, err := os.MkdirTemp(os.Getenv("GOTMPDIR"), "gotest-audit-base-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	tree := filepath.Join(dir, "tree")
	if err := exportCommit(ctx, top, commit, tree); err != nil {
		return nil, fmt.Errorf("exporting %s: %w", ref, err)
	}
	keepWorkspace(top, tree)

	out := filepath.Join(dir, "base.json")
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := append([]string{"audit", "--record", "--output=" + out}, loadFlags...)
	cmd := exec.CommandContext(ctx, exe, append(args, patterns...)...) //nolint:gosec // G204: this CLI, recording the exported tree
	cmd.Dir = filepath.Join(tree, filepath.FromSlash(prefix))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if ctx.Err() != nil {
			return nil, errors.New("interrupted while recording the base")
		}
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			return nil, fmt.Errorf("recording %s: %w\n%s", ref, err, stderr.String())
		}
	}
	rec, err := readRecordFile(out)
	if err != nil {
		return nil, fmt.Errorf("recording %s: %w\n%s", ref, err, stderr.String())
	}
	clean := false
	rec.Provenance = gotestaudit.Provenance{Ref: ref, Commit: short, Dirty: &clean}
	return rec, nil
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
// exported tree that has none, so its modules resolve as the checkout's do.
func keepWorkspace(top, tree string) {
	for _, name := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(filepath.Join(tree, name)); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(top, name)) //nolint:gosec // G304: the checkout's own workspace file
		if err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(tree, name), data, 0o600)
	}
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
