package cli_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIProcsTestSuite: how many suite processes a run starts at once.
type CLIProcsTestSuite struct{}

func (s *CLIProcsTestSuite) SuiteConfig() gotest.SuiteConfig {
	// The failing shape waits three seconds per suite, one after another,
	// after a build under GOMAXPROCS=1.
	return gotest.SuiteConfig{Timeout: 90 * time.Second}
}

func (s *CLIProcsTestSuite) TestSuiteProcessesAtOnce(t *gotest.T) {
	run := func(args ...string) testkit.Run {
		env := []string{"GOMAXPROCS=1", "PROCBARRIER_DIR=" + t.TempDir(), protocol.EnvCacheDir + "=" + t.TempDir()}
		r, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), env, "../testdata/procbarrier/", args...)
		gotest.NoError(t, err)
		return r
	}
	t.When("three suites that each wait for the other two are run with -p 3 and GOMAXPROCS=1", func(w *gotest.T) {
		w.It("all three pass", func(it *gotest.T) {
			r := run("-p", "3")
			gotest.Equal(it, 0, r.Exit, "stdout: %s", r.Stdout)
		})
	})
	t.When("the same suites are run with GOMAXPROCS=1 and no -p", func(w *gotest.T) {
		w.It("each fails waiting, since one process runs at a time", func(it *gotest.T) {
			r := run()
			gotest.Equal(it, 1, r.Exit)
			gotest.Equal(it, "fail", r.Verdict("TestMeetATestSuite/TestMeets"))
		})
	})
}
