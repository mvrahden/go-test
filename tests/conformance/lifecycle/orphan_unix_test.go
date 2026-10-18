//go:build unix

package lifecycle_test

import (
	"os/exec"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OrphansTestSuite: a run leaves no process of its suites behind.
type OrphansTestSuite struct{}

func (s *OrphansTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *OrphansTestSuite) TestSleepLeftBehind(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/orphan/")
	gotest.NoError(t, err)
	t.When("a test starts sleep 4242 and never waits for it", func(w *gotest.T) {
		w.It("the test passes and no such process outlives the run", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestOrphanTestSuite/TestLeavesAProcess"), "stdout: %s", run.Stdout)
			gotest.Eventually(it, 5*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
				gotest.Empty(poll, sleepers(), "sleep 4242 outlived the run")
			})
		})
	})
}

// sleepers lists the pids of every `sleep 4242` on the machine.
func sleepers() []string {
	out, _ := exec.Command("pgrep", "-f", "^sleep 4242$").Output()
	return strings.Fields(string(out))
}
