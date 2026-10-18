package lifecycle_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeadlineNamingTestSuite: an expired --timeout names the tests it cut
// short, and no test that ended before it.
type DeadlineNamingTestSuite struct{}

func (s *DeadlineNamingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *DeadlineNamingTestSuite) TestACrashBeforeTheTimeout(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/lcbcrash/", "--timeout=30s")
	gotest.NoError(t, err)
	named := map[string]bool{}
	for _, ev := range run.Events {
		if ev.Action == "output" && strings.Contains(ev.Output, "--timeout expired while this test was running") {
			named[ev.Test] = true
		}
	}

	t.When("one suite's process crashed on its own and another suite hangs until --timeout", func(w *gotest.T) {
		w.It("names only the hung test as running when the timeout expired", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestCrashTestSuite/TestCrash"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, map[string]bool{"TestHangTestSuite/TestHang": true}, named, "stdout: %s", run.Stdout)
		})
	})
}
