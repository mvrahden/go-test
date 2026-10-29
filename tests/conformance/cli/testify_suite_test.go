package cli_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLITestifyTestSuite: a package part way through a migration runs, with its
// testify suites left to go test.
type CLITestifyTestSuite struct{}

func (s *CLITestifyTestSuite) TestBesideGotestSuites(t *gotest.T) {
	t.When("a package holds a testify suite beside a gotest suite", func(w *gotest.T) {
		root, err := testkit.RepoRoot()
		gotest.NoError(w, err)
		dir := w.TempDir()
		gotest.NoError(w, testkit.StageModule(root, filepath.Join("..", "testdata", "climixedtestify"), dir))
		run, err := testkit.RunCLI(w.Context(), dir, "-v", "./mixed/")
		gotest.NoError(w, err)

		w.It("runs the gotest suite, notes the testify runner as a stdlib test go test runs, and exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout, "--- PASS: TestMigratedTestSuite/TestMigrated ")
			gotest.Contains(it, run.Stderr, "note: 1 stdlib test(s) in climixedtestifymod/mixed not run")
		})
	})
}
