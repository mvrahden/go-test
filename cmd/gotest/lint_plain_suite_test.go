package main_test

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LintPlainTestSuite covers lint without driver flags: findings as text on
// stderr, from one load of the targets.
// Sequential: Setenv.
type LintPlainTestSuite struct{}

// SuiteConfig: every case loads and type-checks a module.
func (s *LintPlainTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *LintPlainTestSuite) TestPlainMode(t *gotest.T) {
	t.When("the target has findings", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		writeLintProbe(w, dir)

		var stderr bytes.Buffer
		code, ok := ExportRunLintPlain(&stderr, dir, []string{"./..."})

		w.It("exits 3 and is handled", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 3, code)
		})
		w.It("prints each finding once, behind its position", func(it *gotest.T) {
			gotest.Regexp(it, `^\S*probe_test\.go:5:1: .+\n$`, stderr.String())
		})
	})

	t.When("the target is clean", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintprobe\n\ngo 1.25\n"), 0o600))
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "probe.go"), []byte("package lintprobe\n\nvar x = 1\n"), 0o600))

		var stderr bytes.Buffer
		code, ok := ExportRunLintPlain(&stderr, dir, []string{"./..."})

		w.It("exits 0 and says nothing", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 0, code)
			gotest.Empty(it, stderr.String())
		})
	})

	t.When("a skip flag disables the violated rule", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		writeLintProbe(w, dir)

		var stderr bytes.Buffer
		code, ok := ExportRunLintPlain(&stderr, dir, []string{"-skip-stdlib-test", "./..."})
		gotest.NoError(w, ExportResetLintSkipFlag("skip-stdlib-test"))

		w.It("honors the flag and reports clean", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 0, code)
			gotest.Empty(it, stderr.String())
		})
	})

	t.When("a driver flag is present", func(w *gotest.T) {
		var stderr bytes.Buffer
		_, ok := ExportRunLintPlain(&stderr, "", []string{"-fix", "./..."})

		w.It("defers to the singlechecker driver", func(it *gotest.T) {
			gotest.False(it, ok)
			gotest.Empty(it, stderr.String())
		})
	})

	t.When("targets do not compile", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintprobe\n\ngo 1.25\n"), 0o600))
		for _, pkg := range []string{"one", "two"} {
			gotest.NoError(w, os.Mkdir(filepath.Join(dir, pkg), 0o700))
			gotest.NoError(w, os.WriteFile(filepath.Join(dir, pkg, "probe.go"), []byte("package "+pkg+"\n\nvar x int = \"nope\"\n"), 0o600))
		}

		var stderr bytes.Buffer
		code, ok := ExportRunLintPlain(&stderr, dir, []string{"./..."})

		w.It("exits 1", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 1, code)
		})
		w.It("names every package nothing was proven about", func(it *gotest.T) {
			gotest.Contains(it, stderr.String(), "FAIL: cannot lint uncompilable packages")
			gotest.Contains(it, stderr.String(), "lintprobe/one\n")
			gotest.Contains(it, stderr.String(), "lintprobe/two\n")
		})
	})
}
