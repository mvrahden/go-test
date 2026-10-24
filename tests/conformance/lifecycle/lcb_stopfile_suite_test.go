package lifecycle_test

import (
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StopFileTestSuite: on Windows the runner stops a suite by its stop file
// alone, so the converter in front of it lives on and the stream ends whole.
// Sequential: it builds the CLI once and drives a live process.
type StopFileTestSuite struct {
	cli string
}

func (s *StopFileTestSuite) SuiteGuard() string {
	if runtime.GOOS != "windows" {
		return "only Windows stops a suite by its stop file alone"
	}
	return ""
}

func (s *StopFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: setup ~45s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{SetupTimeout: 90 * time.Second}
}

func (s *StopFileTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *StopFileTestSuite) AfterAll(t *gotest.T) {}

func (s *StopFileTestSuite) TestOnlyTheStopFileStopsTheSuite(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-json", "../testdata/stopping/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := holding(t, cmd, markerDir)
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	run := finished(cmd, tree, markerDir)

	t.When("a run is interrupted on Windows, where only the stop file reaches the suite", func(w *gotest.T) {
		w.It("skips the methods that have not started with the reason run stopping", func(it *gotest.T) {
			for _, method := range []string{"TestSecond", "TestThird"} {
				path := "TestStopTestSuite/" + method
				gotest.Equal(it, "skip", run.Verdict(path), "verdict of %s; stdout: %s", path, run.Stdout)
				var output strings.Builder
				for _, ev := range run.Events {
					if ev.Test == path && ev.Action == "output" {
						output.WriteString(ev.Output)
					}
				}
				gotest.Contains(it, output.String(), "run stopping")
			}
		})
		w.It("every verdict reaches the stream: the holding method's, the suite's and the package's", func(it *gotest.T) {
			for _, path := range []string{"TestStopTestSuite/TestHolds", "TestStopTestSuite", "-"} {
				gotest.NotEmpty(it, run.Verdict(path), "verdict of %s; stdout: %s", path, run.Stdout)
			}
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
	})
}
