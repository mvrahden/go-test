package main_test

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LintGitHubTestSuite covers the GitHub-annotation mode of the lint
// subcommand: arming, rendering, step summary, and driver fallback.
// Not parallel: Setenv is incompatible with parallel subtests.
type LintGitHubTestSuite struct{}

// writeLintProbe materializes a one-file module whose stdlib-style test
// triggers the stdlib-test rule without importing gotest.
func writeLintProbe(t *gotest.T, dir string) {
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintprobe\n\ngo 1.24\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "probe_test.go"), []byte("package lintprobe\n\nimport \"testing\"\n\nfunc TestProbe(t *testing.T) {\n\tt.Log(\"probe\")\n}\n"), 0o600))
}

func (s *LintGitHubTestSuite) TestArming(t *gotest.T) {
	t.When("outside GitHub Actions", func(w *gotest.T) {
		w.Setenv("GITHUB_ACTIONS", "")

		w.It("stays off without the flag", func(it *gotest.T) {
			gotest.False(it, ExportLintGitHubArmed([]string{"./..."}))
		})

		w.It("arms with --github", func(it *gotest.T) {
			gotest.True(it, ExportLintGitHubArmed([]string{"--github", "./..."}))
		})
	})

	t.When("inside GitHub Actions", func(w *gotest.T) {
		w.Setenv("GITHUB_ACTIONS", "true")

		w.It("auto-arms without the flag", func(it *gotest.T) {
			gotest.True(it, ExportLintGitHubArmed([]string{"./..."}))
		})
	})
}

func (s *LintGitHubTestSuite) TestGitHubMode(t *gotest.T) {
	t.When("the target has findings", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		summaryPath := filepath.Join(w.TempDir(), "step_summary.md")
		w.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
		dir := w.TempDir()
		writeLintProbe(w, dir)

		var stdout, stderr bytes.Buffer
		code, ok := ExportRunLintGitHub(&stdout, &stderr, dir, []string{"./..."})

		w.It("exits 3 and is handled", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 3, code)
		})

		w.It("emits a module-relative annotation with rule title and column", func(it *gotest.T) {
			gotest.Contains(it, stdout.String(), "::error file=probe_test.go,line=")
			gotest.Contains(it, stdout.String(), ",col=")
			gotest.Contains(it, stdout.String(), ",title=stdlib-test::")
		})

		w.It("keeps findings off stderr so problem matchers cannot double-annotate", func(it *gotest.T) {
			gotest.Empty(it, stderr.String())
		})

		w.It("appends the step summary", func(it *gotest.T) {
			content, err := os.ReadFile(summaryPath)
			gotest.NoError(it, err)
			gotest.Contains(it, string(content), "stdlib-test")
			gotest.Contains(it, string(content), "probe_test.go")
		})
	})

	t.When("the target is clean", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		summaryPath := filepath.Join(w.TempDir(), "step_summary.md")
		w.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
		dir := w.TempDir()
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintprobe\n\ngo 1.24\n"), 0o600))
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "probe.go"), []byte("package lintprobe\n\nvar x = 1\n"), 0o600))

		var stdout, stderr bytes.Buffer
		code, ok := ExportRunLintGitHub(&stdout, &stderr, dir, []string{"./..."})

		w.It("exits 0 with no annotations and no step summary", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 0, code)
			gotest.Empty(it, stdout.String())
			_, statErr := os.Stat(summaryPath)
			gotest.True(it, os.IsNotExist(statErr), "step summary should not be written when clean")
		})
	})

	t.When("a skip flag disables the violated rule", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		w.Setenv("GITHUB_STEP_SUMMARY", "")
		dir := w.TempDir()
		writeLintProbe(w, dir)

		var stdout, stderr bytes.Buffer
		code, ok := ExportRunLintGitHub(&stdout, &stderr, dir, []string{"-skip-stdlib-test", "./..."})
		gotest.NoError(w, ExportResetLintSkipFlag("skip-stdlib-test"))

		w.It("honors the flag and reports clean", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 0, code)
			gotest.Empty(it, stdout.String())
		})
	})

	t.When("an unsupported driver flag is present", func(w *gotest.T) {
		var stdout, stderr bytes.Buffer
		_, ok := ExportRunLintGitHub(&stdout, &stderr, "", []string{"-json", "./..."})

		w.It("defers to the singlechecker driver", func(it *gotest.T) {
			gotest.False(it, ok)
			gotest.Empty(it, stdout.String())
		})
	})

	t.When("the target does not compile", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintprobe\n\ngo 1.24\n"), 0o600))
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "probe.go"), []byte("package lintprobe\n\nvar x int = \"nope\"\n"), 0o600))

		var stdout, stderr bytes.Buffer
		code, ok := ExportRunLintGitHub(&stdout, &stderr, dir, []string{"./..."})

		w.It("fails loudly instead of passing silently", func(it *gotest.T) {
			gotest.True(it, ok)
			gotest.Equal(it, 1, code)
			gotest.Contains(it, stderr.String(), "FAIL")
		})
	})
}
