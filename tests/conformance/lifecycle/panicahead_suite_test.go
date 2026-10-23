package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PanicAheadTestSuite: a panic ahead of an AfterEach that fails is still
// reported.
type PanicAheadTestSuite struct{}

func (s *PanicAheadTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

func (s *PanicAheadTestSuite) TestPanicAheadOfAFailingAfterEach(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/lcpanicahead/")
	gotest.NoError(t, err)
	t.When("a test method's BeforeEach panics and its AfterEach fails a fatal assertion", func(w *gotest.T) {
		w.It("the test fails with the panic reported, and its AfterEach still runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestPanicAheadTestSuite/TestOne"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout+run.Stderr, "panic: TEST-BEFORE-EACH-PANIC")
			gotest.Contains(it, run.Stdout, "MARK after the panic")
		})
	})
}
