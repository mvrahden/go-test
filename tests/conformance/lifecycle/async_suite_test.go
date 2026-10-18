package lifecycle_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AsyncMethodsTestSuite: an async method waits for done, for a declared
// budget at most, and no longer once nothing can call done.
type AsyncMethodsTestSuite struct{}

func (s *AsyncMethodsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AsyncMethodsTestSuite) TestAFatalGoroutine(t *gotest.T) {
	began := time.Now()
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/asyncmethods/")
	gotest.NoError(t, err)
	t.When("the goroutine that owns done fails a fatal assertion", func(w *gotest.T) {
		w.It("fails the method at once, not at the end of its budget", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestAsyncTestSuite/TestFatalGoroutineAsync"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestAsyncTestSuite/TestCallsDoneAsync"))
			gotest.Less(it, time.Since(began), 10*time.Second)
		})
	})
}
