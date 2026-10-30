package cli_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIScaffoldPathsTestSuite: scaffold takes a file or a package by any path
// an editor would hand it, relative or absolute.
type CLIScaffoldPathsTestSuite struct{}

func (s *CLIScaffoldPathsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *CLIScaffoldPathsTestSuite) TestFilePaths(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	t.When("a source file is named by path", func(w *gotest.T) {
		w.It("scaffolds beside the file, by a relative or an absolute path", func(it *gotest.T) {
			for sub, tc := range gotest.Each(it, []struct {
				Desc     string
				absolute bool
			}{
				{"a relative path scaffolds beside the file", false},
				{"an absolute path scaffolds beside the file", true},
			}) {
				dir := sub.TempDir()
				gotest.NoError(sub, testkit.WriteModule(root, dir, "example.com/scaf"))
				gotest.NoError(sub, os.WriteFile(filepath.Join(dir, "calc.go"), []byte(scafPackageSrc), 0o600))
				target := "./calc.go"
				if tc.absolute {
					target = filepath.Join(dir, "calc.go")
				}
				run, err := testkit.RunCLI(sub.Context(), dir, "scaffold", target)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				_, err = os.Stat(filepath.Join(dir, "calc_suite_test.go"))
				gotest.NoError(sub, err, "the scaffold was not written beside the file")
			}
		})
	})
}

// scafPackageSrc is the package the package-path cases scaffold from.
const scafPackageSrc = "package scaf\n\n// Calc adds.\ntype Calc struct{}\n\n// Add returns a + b.\nfunc (c Calc) Add(a, b int) int { return a + b }\n\n// Render formats n.\nfunc Render(n int) string { return \"\" }\n"

func (s *CLIScaffoldPathsTestSuite) TestPackagePaths(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	t.When("a type or a function is named by an absolute package path from outside its module", func(w *gotest.T) {
		w.It("scaffolds into the package's directory", func(it *gotest.T) {
			for sub, tc := range gotest.Each(it, []struct {
				Desc    string
				args    []string
				written string
			}{
				{"a type", []string{".Calc"}, "calc_suite_test.go"},
				{"a function to fuzz", []string{"--fuzz", ".Render"}, "render_fuzz_test.go"},
			}) {
				dir := sub.TempDir()
				gotest.NoError(sub, testkit.WriteModule(root, dir, "example.com/scaf"))
				gotest.NoError(sub, os.WriteFile(filepath.Join(dir, "calc.go"), []byte(scafPackageSrc), 0o600))
				args := append([]string{"scaffold"}, tc.args...)
				args[len(args)-1] = dir + args[len(args)-1]
				run, err := testkit.RunCLI(sub.Context(), sub.TempDir(), args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				_, err = os.Stat(filepath.Join(dir, tc.written))
				gotest.NoError(sub, err, "the scaffold was not written into the package's directory")
			}
		})
	})
}
