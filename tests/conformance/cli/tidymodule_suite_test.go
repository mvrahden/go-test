package cli_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLITidyModuleTestSuite: a module that only requires gotest runs its suites
// with the go.mod go mod tidy leaves it and no go.sum.
type CLITidyModuleTestSuite struct{}

func (s *CLITidyModuleTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute}
}

func (s *CLITidyModuleTestSuite) TestWithoutTheToolDirective(t *gotest.T) {
	t.When("a tidied module requires gotest and declares no tool directive", func(w *gotest.T) {
		root, err := testkit.RepoRoot()
		gotest.NoError(w, err)
		dir := w.TempDir()
		gotest.NoError(w, testkit.StageModule(root, filepath.Join("..", "testdata", "clitidymodule"), dir))
		// The staged go.work would lend the module this repository's go.sum.
		run, err := testkit.RunCLIWithEnv(w.Context(), dir, []string{"GOWORK=off", "GOFLAGS="}, "-v", "./calc/")
		gotest.NoError(w, err)

		w.It("runs its suites and exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout, "--- PASS: TestCalcTestSuite/TestAdds ")
			_, err := os.Stat(filepath.Join(dir, "go.sum"))
			gotest.ErrorIs(it, err, fs.ErrNotExist, "the run needed a go.sum the tidied module does not have")
		})
	})
}
