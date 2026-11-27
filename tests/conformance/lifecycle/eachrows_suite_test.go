package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachRowsTestSuite: every row of an Each table is its own subtest with its
// own verdict; a failing assertion in a row, a helper's, a deadline T's or
// a poll's included, lets the row and the next row run, a row that leaves
// early ends the method there, and a row that panics after it failed ends its
// method, not the suite.
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
	t.When("a row of an Each table panics after an assertion on its T failed", func(w *gotest.T) {
		const method = "TestEachRowsTestSuite/TestARowPanicsAfterFailing"
		w.It("fails that row with the panic's value at the line that panicked, and prints no stack", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict(method+"/first"))
			gotest.Equal(it, "fail", run.Verdict(method+"/second"))
			gotest.Contains(it, run.Stdout, "eachrows_suite_test.go:135: panic after the row failed: runtime error: invalid memory address or nil pointer dereference")
			gotest.NotContains(it, run.Stdout, "goroutine ")
			gotest.NotContains(it, run.Stdout+run.Stderr, "recovered from Each entry")
		})
		w.It("ends the method there as failed, naming the row and how many rows after it did not run", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict(method))
			gotest.Empty(it, run.Verdict(method+"/third"))
			gotest.Contains(it, run.Markers, "panic second")
			gotest.NotContains(it, run.Markers, "panic third")
			gotest.NotContains(it, run.Markers, "after the panic")
			gotest.Contains(it, run.Stdout, "eachrows_suite_test.go:132: ended at row second: it panicked after it failed; rows after it not run: 2")
		})
		w.It("runs AfterEach, the method after it and AfterAll", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("after each "+method, "the method after the panics", "after all"), "markers: %v", run.Markers)
			gotest.Equal(it, "pass", run.Verdict("TestEachRowsTestSuite/TestTheMethodAfterThePanics"))
		})
	})
	t.When("a row panics after it failed in a helper given its T, on NewTWithDeadline of it, in Eventually given it or in an It on it", func(w *gotest.T) {
		w.It("the method ends there as failed and the method after it runs", func(it *gotest.T) {
			for method, value := range map[string]string{
				"TestPanicAfterAHelper":    "after the helper",
				"TestPanicAfterADeadline":  "after the deadline T",
				"TestPanicAfterEventually": "after Eventually",
				"TestPanicAfterAnIt":       "after the It",
			} {
				gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/"+method+"/first"), method)
				gotest.Equal(it, "fail", run.Verdict("TestEachRowsTestSuite/"+method), method)
				gotest.Contains(it, run.Stdout, "panic after the row failed: "+value)
				gotest.Contains(it, run.Markers, "after each TestEachRowsTestSuite/"+method)
			}
			gotest.Contains(it, run.Stdout, "ended at row first: it panicked after it failed\\n")
			gotest.Equal(it, "pass", run.Verdict("TestEachRowsTestSuite/TestTheMethodAfterThePanics"))
		})
	})
}
