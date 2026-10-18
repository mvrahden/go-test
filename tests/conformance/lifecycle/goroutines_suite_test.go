package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoroutinesTestSuite: where the wait on a goroutine started by gotest.Go
// ends, and what it ends with.
type GoroutinesTestSuite struct{}

func (s *GoroutinesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *GoroutinesTestSuite) TestWaits(t *gotest.T) {
	// A wait that never ends is beyond the runner's graceful stop; go test's own
	// alarm bounds it.
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/goroutines/", "-timeout=20s")
	gotest.NoError(t, err)
	t.When("BeforeAll starts a goroutine with gotest.Go that only AfterAll stops", func(w *gotest.T) {
		w.It("the suite passes: the wait comes after AfterAll", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestServerTestSuite"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
	t.When("a goroutine started by gotest.Go fails a fatal assertion", func(w *gotest.T) {
		w.It("the wait on it fails the method there, and nothing after the wait runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestGoexitTestSuite/TestGoexit"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "from the goroutine")
			gotest.NotContains(it, run.Stdout, "the method went on after the wait")
		})
	})
}
