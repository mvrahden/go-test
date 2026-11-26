package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachRowsTestSuite: every row of an Each table is its own subtest with its
// own verdict; a failing assertion in a row, a helper's, a deadline T's or
// a poll's included, lets the row and the next row run, and a row that leaves
// early ends the method there.
type EachRowsTestSuite struct{}

func (s *EachRowsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *EachRowsTestSuite) TestRowsAreSubtests(t *gotest.T) {
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
			gotest.Contains(it, run.Markers, "after the loop")
			gotest.Regexp(it, `eachrows_suite_test\.go:\d+: True failed`, run.Stdout)
		})
	})
	t.When("an assertion fails in a helper an Each row hands its T", func(w *gotest.T) {
		w.It("the failure names the helper's line, and the row goes on", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestAHelperInARow/first"))
			gotest.Regexp(it, `eachrows_suite_test\.go:90: eachrows_suite_test\.go:\d+: True failed`, run.Stdout)
			gotest.Contains(it, run.Markers, "after the helper")
		})
	})
	t.When("an It runs on an Each row's T", func(w *gotest.T) {
		w.It("an assertion that fails in its body ends the It there, and the row goes on", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestAnItInARow/first/stops"))
			gotest.NotContains(it, run.Markers, "after the failure in the It")
			gotest.Contains(it, run.Markers, "after the It")
		})
	})
	t.When("an assertion fails on NewTWithDeadline of an Each row's T", func(w *gotest.T) {
		w.It("fails the row, and the row goes on", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestADeadlineInARow/first"))
			gotest.Contains(it, run.Stdout, "message: the deadline T fails")
			gotest.Contains(it, run.Markers, "after the deadline T")
		})
	})
	t.When("Eventually is given an Each row's T and no poll passes", func(w *gotest.T) {
		w.It("fails the row, and the row goes on", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestEventuallyInARow/first"))
			gotest.Regexp(it, `Eventually failed after 50ms`, run.Stdout)
			gotest.Contains(it, run.Markers, "after Eventually")
		})
	})
	t.When("a row of an Each table calls FailNow", func(w *gotest.T) {
		w.It("fails that row and the method there, and the rows after it never run", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestEachRowsTestSuite/TestAStoppingRow/first"))
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestAStoppingRow/second"))
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestAStoppingRow"))
			gotest.Contains(it, run.Markers, "stop second")
			gotest.NotContains(it, run.Markers, "stop third")
			gotest.NotContains(it, run.Markers, "after the stop")
		})
	})
	t.When("one row of an Each table skips", func(w *gotest.T) {
		w.It("skips that row and the method from there, and the rows after it never run", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestEachRowsTestSuite/TestASkippingRow/first"))
			gotest.Equal(it, "skip", run.Verdict("TestEachRowsTestSuite/TestASkippingRow/second"))
			gotest.Equal(it, "skip", run.Verdict("TestEachRowsTestSuite/TestASkippingRow"), "stdout: %s", run.Stdout)
			gotest.NotContains(it, run.Markers, "skip third")
			gotest.NotContains(it, run.Markers, "after the skip")
			gotest.Regexp(it, `eachrows_suite_test\.go:\d+: skipped at row second`, run.Stdout)
		})
	})
	t.When("a row of an Each table skips or fatally fails the method's own T", func(w *gotest.T) {
		w.It("the method skips or fails there, with no failure of its own", func(it *gotest.T) {
			gotest.Equal(it, "skip", run.Verdict("TestEachRowsTestSuite/TestMethodSkipsInARow"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/TestMethodFailsInARow"))
			gotest.NotContains(it, run.Markers, "method skip second")
			gotest.NotContains(it, run.Markers, "method fail second")
			gotest.NotContains(it, run.Stdout, "without a verdict")
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
