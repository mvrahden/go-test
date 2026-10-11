package main_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// LintFixTestSuite runs the built CLI's lint -fix, which applies fixes in a
// child driver and then reports what is left.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type LintFixTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *LintFixTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *LintFixTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const lintFixProbe = `package lintfix

import "github.com/mvrahden/go-test/pkg/gotest"

type ProbeTestSuite struct{}

func (s *ProbeTestSuite) TestProbe(t *gotest.T) {
	t.T().Helper()
	gotest.True(t, 1 == 1)
}
`

// runLintIn runs the binary in dir and returns stdout, stderr and exit code.
func (s *LintFixTestSuite) runLintIn(t *gotest.T, dir string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(s.cli.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PWD="+dir)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, errs.String())
	return out.String(), errs.String(), cmd.ProcessState.ExitCode()
}

func (s *LintFixTestSuite) stageProbe(t *gotest.T, src string) (dir, file string) {
	dir = t.TempDir()
	gotest.NoError(t, testkit.WriteModule(s.cli.repoRoot, dir, "lintfix"))
	file = filepath.Join(dir, "probe_test.go")
	gotest.NoError(t, os.WriteFile(file, []byte(src), 0o600))
	return dir, file
}

func (s *LintFixTestSuite) TestFix(t *gotest.T) {
	t.When("a finding has no fix", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		stdout, stderr, code := s.runLintIn(w, dir, "lint", "-fix", "./...")
		src, err := os.ReadFile(file)
		gotest.NoError(w, err)

		w.It("applies the fixable rewrite", func(it *gotest.T) {
			gotest.Contains(it, string(src), "gotest.Equal(t, 1, 1)")
		})
		w.It("reports the finding it could not fix", func(it *gotest.T) {
			gotest.Regexp(it, `^\S*probe_test\.go:8:2: never call Helper .+\n$`, stderr)
			gotest.Empty(it, stdout)
		})
		w.It("exits 3", func(it *gotest.T) {
			gotest.Equal(it, 3, code, stderr)
		})
	})

	t.When("every finding has a fix", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		src, err := os.ReadFile(file)
		gotest.NoError(w, err)
		gotest.NoError(w, os.WriteFile(file, bytes.Replace(src, []byte("\tt.T().Helper()\n"), nil, 1), 0o600))

		stdout, stderr, code := s.runLintIn(w, dir, "lint", "-fix", "./...")

		w.It("exits 0 and says nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, code, stderr)
			gotest.Empty(it, stderr)
			gotest.Empty(it, stdout)
		})
	})
}
