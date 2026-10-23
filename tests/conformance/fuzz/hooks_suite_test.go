package fuzz_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzHooksTestSuite: a fuzz execution's hooks pair the way a test method's
// do.
type FuzzHooksTestSuite struct{}

func (s *FuzzHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

func (s *FuzzHooksTestSuite) TestBeforeEachLeaves(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fuzzhooks/")
	gotest.NoError(t, err)
	markers := run.Markers
	t.When("a fuzz execution's BeforeEach fails", func(w *gotest.T) {
		w.It("the execution fails and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("FuzzBrokenBeforeTestSuite_FuzzOne/seed#0"), "stdout: %s", run.Stdout)
			gotest.Contains(it, markers, "after broken")
		})
	})
	t.When("a fuzz execution's BeforeEach panics", func(w *gotest.T) {
		w.It("the execution fails and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("FuzzPanicBeforeTestSuite_FuzzOne/seed#0"), "stdout: %s", run.Stdout)
			gotest.Contains(it, markers, "after panic")
		})
	})
	t.When("a fuzz execution's BeforeEach panics and its AfterEach fails a fatal assertion", func(w *gotest.T) {
		w.It("the execution fails with the panic reported, and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("FuzzPanicAheadTestSuite_FuzzOne/seed#0"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout+run.Stderr, "panic: FUZZ-BEFORE-EACH-PANIC")
			gotest.Contains(it, markers, "after the panic")
		})
	})
}
