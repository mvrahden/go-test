package cli_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIScaffoldPathsTestSuite: scaffold takes a file by any path an editor
// would hand it, relative or absolute.
type CLIScaffoldPathsTestSuite struct{}

func (s *CLIScaffoldPathsTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
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
				gotest.NoError(sub, os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package scaf\n\n// Calc adds.\ntype Calc struct{}\n\n// Add returns a + b.\nfunc (c Calc) Add(a, b int) int { return a + b }\n"), 0o600))
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
