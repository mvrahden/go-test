//go:build unix

package cli_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// A temp directory on macOS is reached through a symlink; the module's
// go.work names the real path, and the two must still be one package.
func (s *CLIScaffoldPathsTestSuite) TestASymlinkedPath(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := t.TempDir()
	gotest.NoError(t, testkit.WriteModule(root, dir, "example.com/scaf"))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package scaf\n\n// Calc adds.\ntype Calc struct{}\n\n// Add returns a + b.\nfunc (c Calc) Add(a, b int) int { return a + b }\n"), 0o600))
	link := filepath.Join(t.TempDir(), "link")
	gotest.NoError(t, os.Symlink(dir, link))
	run, err := testkit.RunCLI(t.Context(), dir, "scaffold", filepath.Join(link, "calc.go"))
	gotest.NoError(t, err)
	t.When("a source file is named by an absolute path through a symlinked directory", func(w *gotest.T) {
		w.It("scaffolds beside the file", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			_, err := os.Stat(filepath.Join(dir, "calc_suite_test.go"))
			gotest.NoError(it, err, "the scaffold was not written beside the file")
		})
	})
}
