package main_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// buildGotestBinary builds the CLI under test into binDir, a temp dir the
// caller took from its own t.TempDir() so the framework removes it.
func buildGotestBinary(t *gotest.T, repoRoot, binDir string) string {
	name := "gotest"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(binDir, name)
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "build gotest binary: %s", string(out))
	return binary
}

// stageFuzzModule copies testdata/fuzzcrash into dir (the caller's fresh
// t.TempDir()), points its replace at repoRoot and writes a go.work beside
// it, so the CLI resolves gotest the way a user's module does — outside the
// repository tree, where no editor watcher or ./... pattern can see it.
func stageFuzzModule(t *gotest.T, repoRoot, dir string) string {
	src := filepath.Join(repoRoot, "cmd", "gotest", "testdata", "fuzzcrash")
	entries, err := os.ReadDir(src)
	gotest.NoError(t, err)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		gotest.NoError(t, err)
		if e.Name() == "go.mod" {
			data = []byte(strings.ReplaceAll(string(data), "REPO_ROOT", repoRoot))
		}
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, e.Name()), data, 0o600))
	}
	work := "go 1.25.0\n\nuse (\n\t.\n\t" + repoRoot + "\n)\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.work"), []byte(work), 0o600))
	return dir
}

// runGotestIn runs the binary in dir and returns its combined output and
// exit code; a nil env keeps cmd.Env nil so exec sets the child's PWD.
func runGotestIn(t *gotest.T, binary, dir string, env []string, args ...string) (string, int) {
	cmd := exec.Command(binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(append(os.Environ(), "PWD="+dir), env...)
	}
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, out)
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return string(out), code
}
