// Package testkit builds and stages what the suites that drive the gotest
// CLI need. It returns errors instead of asserting, so ring-0 suites may use it.
package testkit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
)

// goLine is the go directive of every staged go.mod and go.work.
const goLine = "go 1.26.0"

// BuildCLI builds repoRoot's ./cmd/gotest into dir and returns the binary's path.
func BuildCLI(ctx context.Context, repoRoot, dir string) (string, error) {
	if cache := os.Getenv("GOTEST_TEST_CLI_CACHE_DIR"); cache != "" {
		return cachedCLI(ctx, repoRoot, dir, cache)
	}
	return buildCLI(ctx, repoRoot, dir)
}

func buildCLI(ctx context.Context, repoRoot, dir string) (string, error) {
	binary := filepath.Join(dir, cliName())
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build gotest: %w\n%s", err, out)
	}
	return binary, nil
}

// cliName is the CLI binary's file name.
func cliName() string {
	if runtime.GOOS == "windows" {
		return "gotest.exe"
	}
	return "gotest"
}

// StageModule copies the module tree at src into dir, points its go.mod's
// REPO_ROOT placeholder at repoRoot and pairs the two in a go.work.
func StageModule(repoRoot, src, dir string) error {
	err := copyTree(src, dir, func(name string, data []byte) []byte {
		if name == "go.mod" {
			return []byte(strings.ReplaceAll(string(data), "REPO_ROOT", repoRoot))
		}
		return data
	})
	if err != nil {
		return err
	}
	return writeGoWork(repoRoot, dir)
}

// StagePackage copies the package directory src into dir under its own base
// name and makes dir a module (see WriteModule), so the package builds under
// plain go test against this checkout. It returns the staged package's path.
func StagePackage(repoRoot, src, dir, module string) (string, error) {
	pkgDir := filepath.Join(dir, filepath.Base(filepath.Clean(src)))
	if err := copyTree(src, pkgDir, nil); err != nil {
		return "", err
	}
	return pkgDir, WriteModule(repoRoot, dir, module)
}

// copyTree copies the files under src into dir, each through edit when given.
func copyTree(src, dir string, edit func(name string, data []byte) []byte) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(path) //nolint:gosec // G122: copies the repo's own fixture into a fresh temp dir
		if err != nil {
			return err
		}
		if edit != nil {
			data = edit(d.Name(), data)
		}
		return os.WriteFile(target, data, 0o600)
	})
}

// WriteModule makes dir a module that requires gotest from repoRoot, with
// the go.work that pairs the two.
func WriteModule(repoRoot, dir, module string) error {
	goMod := "module " + module + "\n\n" + goLine + "\n\nrequire github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000\n\nreplace github.com/mvrahden/go-test => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
		return err
	}
	return writeGoWork(repoRoot, dir)
}

func writeGoWork(repoRoot, dir string) error {
	work := goLine + "\n\nuse (\n\t.\n\t" + repoRoot + "\n)\n"
	return os.WriteFile(filepath.Join(dir, "go.work"), []byte(work), 0o600)
}

// ScrubActionsEnv removes the GitHub Actions variables from this process.
// Every child CLI inherits them; under CI each would append to the job's
// real step summary.
func ScrubActionsEnv() {
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("GITHUB_STEP_SUMMARY")
}

// PrivateCache points this process's gotest cache at dir, for every child
// CLI that inherits the environment. A run from a fresh directory never hits
// a cache entry, so it must not fill the user's; dir should go with the run.
func PrivateCache(dir string) error {
	return os.Setenv(protocol.EnvCacheDir, dir)
}

// StartCLI starts cmd as the root of its own process tree, so Interrupt can
// reach it on every platform.
func StartCLI(cmd *exec.Cmd) (*proctree.Tree, error) {
	tree := proctree.New(cmd)
	return tree, tree.Start()
}

// Interrupt sends a CLI started by StartCLI the interrupt a terminal sends:
// SIGINT on Unix. Windows has no signal for one process, so the tree's console
// gets CTRL_BREAK, which Go also delivers as os.Interrupt.
func Interrupt(cmd *exec.Cmd, tree *proctree.Tree) error {
	if runtime.GOOS == "windows" {
		return tree.Interrupt()
	}
	return cmd.Process.Signal(os.Interrupt)
}
