package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachRowsTestSuite: every row of an Each table is its own subtest; one
// failing row fails the method, not the table.
type EachRowsTestSuite struct{}

func (s *EachRowsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *EachRowsTestSuite) TestRowsAreIsolated(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/eachrows/")
	gotest.NoError(t, err)
	t.When("one row of an Each table fails", func(w *gotest.T) {
		w.It("fails that row and the method, and the rows after it still run", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestEachRowsTestSuite/TestAFailingRow/first"))
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestAFailingRow/second"))
			gotest.Equal(it, "pass", run.Verdict("TestEachRowsTestSuite/TestAFailingRow/third"))
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestAFailingRow"))
			gotest.Contains(it, run.Markers, "row third")
		})
		w.It("runs the rest of the method after the table", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "after the loop")
		})
	})
	t.When("the loop over an Each table breaks", func(w *gotest.T) {
		w.It("stops at the break and the method goes on", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestEachRowsTestSuite/TestBreak"))
			gotest.Contains(it, run.Markers, "break first")
			gotest.NotContains(it, run.Markers, "break second")
			gotest.Contains(it, run.Markers, "after the break")
		})
	})
}
