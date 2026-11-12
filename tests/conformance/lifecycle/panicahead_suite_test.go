package lifecycle_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PanicAheadTestSuite: a panic ahead of an AfterEach that fails is still
// reported.
type PanicAheadTestSuite struct{}

func (s *PanicAheadTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
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

func (s *PanicAheadTestSuite) TestAMethodsPanicIsPrintedOnce(t *gotest.T) {
	t.When("a test method panics", func(w *gotest.T) {
		run, err := testkit.RunPackageWithEnv(w.Context(), w.TempDir(), []string{"HOOKS_AT=body", "HOOKS_MODE=panic"}, "../testdata/hooks/")
		gotest.NoError(w, err)
		w.It("the report names the line it happened at, and its stack shows that line once", func(it *gotest.T) {
			line := lineOf(it, "hooks_suite_test.go", `panic(hook + " panicked")`)
			var out strings.Builder
			for _, ev := range run.Events {
				out.WriteString(ev.Output)
			}
			all := out.String() + run.Stderr
			gotest.Equal(it, 1, run.Exit, all)
			gotest.Contains(it, all, "[panicked at hooks_suite_test.go:"+line+"; recovered from the test method, ahead of its AfterEach]")
			gotest.Equal(it, 1, strings.Count(all, "testdata/hooks/hooks_suite_test.go:"+line+" +0x"), all)
		})
	})
}
