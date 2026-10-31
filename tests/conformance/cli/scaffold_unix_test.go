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
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "calc.go"), []byte(scafPackageSrc), 0o600))
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

func (s *CLIScaffoldPathsTestSuite) TestASymlinkedPackagePath(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := t.TempDir()
	gotest.NoError(t, testkit.WriteModule(root, dir, "example.com/scaf"))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "calc.go"), []byte(scafPackageSrc), 0o600))
	link := filepath.Join(t.TempDir(), "link")
	gotest.NoError(t, os.Symlink(dir, link))
	run, err := testkit.RunCLI(t.Context(), dir, "scaffold", link+".Calc")
	gotest.NoError(t, err)
	t.When("a type is named by an absolute package path through a symlinked directory", func(w *gotest.T) {
		w.It("scaffolds into the package's directory", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			_, err := os.Stat(filepath.Join(dir, "calc_suite_test.go"))
			gotest.NoError(it, err, "the scaffold was not written into the package's directory")
		})
	})
}

func (s *CLIScaffoldPathsTestSuite) TestASymlinkedWorkingDirectory(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := t.TempDir()
	gotest.NoError(t, testkit.WriteModule(root, dir, "example.com/scaf"))
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "calc"), 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "calc", "calc.go"), []byte(scafPackageSrc), 0o600))
	link := filepath.Join(t.TempDir(), "link")
	gotest.NoError(t, os.Symlink(dir, link))
	run, err := testkit.RunCLI(t.Context(), link, "scaffold", "./calc.Calc")
	gotest.NoError(t, err)
	t.When("a type is named by a relative package path from a symlinked working directory", func(w *gotest.T) {
		w.It("scaffolds into the package's directory, named relative to the working directory", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "Generated: calc/calc_suite_test.go\n", run.Stdout)
			_, err := os.Stat(filepath.Join(dir, "calc", "calc_suite_test.go"))
			gotest.NoError(it, err, "the scaffold was not written into the package's directory")
		})
	})
}
