package discovery_test

import (
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CaselessTestSuite: a type named like a suite declares a case; one that
// declares none is refused, and a helper is named otherwise.
type CaselessTestSuite struct{}

func (s *CaselessTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const caselessRefusal = `suite HelperTestSuite declares no Test, Benchmark or Fuzz method; declare one, or give the helper type a name that does not end in TestSuite`

func (s *CaselessTestSuite) TestCaselessSuites(t *gotest.T) {
	t.When("a type named like a suite declares no case", func(w *gotest.T) {
		w.It("the suite is refused at its declaration, told to declare a case or name the helper otherwise", func(it *gotest.T) {
			checkShape(it, shape{pkg: "caseless", exit: 2, shows: `caseless_suite_test\.go:6:6: ` + caselessRefusal})
		})
		w.It("discover reports the package broken with the refusal", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "discover", "../testdata/shapes/caseless/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout, `"broken":true,"suites":[]`)
			gotest.Contains(it, run.Stdout, `"file":"caseless_suite_test.go","line":6,"col":6,"message":"`+caselessRefusal+`"`)
		})
		w.It("a suite whose only case is excluded runs as skipped", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/excludedonly/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "skip", run.Verdict("TestLaterTestSuite"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestRealTestSuite/TestRuns"), "stdout: %s", run.Stdout)
		})
		w.It("a testify base suite that declares no case is left to testify", func(it *gotest.T) {
			root, err := testkit.RepoRoot()
			gotest.NoError(it, err)
			dir := it.TempDir()
			gotest.NoError(it, testkit.StageModule(root, filepath.Join("..", "testdata", "climixedtestify"), dir))
			run, err := testkit.RunCLI(it.Context(), dir, "-v", "./mixed/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout, "--- PASS: TestMigratedTestSuite/TestMigrated ")
		})
	})
}
