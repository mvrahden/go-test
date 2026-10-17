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
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *CLIScaffoldPathsTestSuite) TestFilePaths(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	t.When("a source file is named by path", func(w *gotest.T) {
		for it, tc := range gotest.Each(w, []struct {
			Desc     string
			absolute bool
		}{
			{"a relative path scaffolds beside the file", false},
			{"an absolute path scaffolds beside the file", true},
		}) {
			dir := it.TempDir()
			gotest.NoError(it, testkit.WriteModule(root, dir, "example.com/scaf"))
			gotest.NoError(it, os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package scaf\n\n// Calc adds.\ntype Calc struct{}\n\n// Add returns a + b.\nfunc (c Calc) Add(a, b int) int { return a + b }\n"), 0o600))
			target := "./calc.go"
			if tc.absolute {
				target = filepath.Join(dir, "calc.go")
			}
			run, err := testkit.RunCLI(it.Context(), dir, "scaffold", target)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			_, err = os.Stat(filepath.Join(dir, "calc_suite_test.go"))
			gotest.NoError(it, err, "the scaffold was not written beside the file")
		}
	})
}
