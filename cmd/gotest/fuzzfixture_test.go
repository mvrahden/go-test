package main_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// stageFuzzModule copies testdata/fuzzcrash into dir (the caller's fresh
// t.TempDir()), so the CLI resolves gotest the way a user's module does —
// outside the repository tree, where no editor watcher or ./... pattern can see it.
func stageFuzzModule(t *gotest.T, repoRoot, dir string) string {
	src := filepath.Join(repoRoot, "cmd", "gotest", "testdata", "fuzzcrash")
	gotest.NoError(t, testkit.StageModule(repoRoot, src, dir))
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
