package fuzz_test

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzSharedFixtureTestSuite: gotest fuzz and gotest fuzz triage start the
// shared fixtures a target's suite binds, and its processes hydrate them.
type FuzzSharedFixtureTestSuite struct{}

func (s *FuzzSharedFixtureTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 90 * time.Second}
}

// runFuzzCLI runs the CLI with args over fzshared and returns the run and its
// markers.
func runFuzzCLI(t *gotest.T, args ...string) (testkit.Run, []string) {
	return runFuzzCLIOn(t, "../testdata/fzshared/", args...)
}

// runFuzzCLIOn is runFuzzCLI over pkg.
func runFuzzCLIOn(t *gotest.T, pkg string, args ...string) (testkit.Run, []string) {
	markerDir := t.TempDir()
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GOTEST_CANARY_DIR=" + markerDir}, append(args, pkg)...)
	gotest.NoError(t, err)
	markers, err := testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
	gotest.NoError(t, err)
	return run, markers
}

func (s *FuzzSharedFixtureTestSuite) TestFuzz(t *gotest.T) {
	t.When("gotest fuzz runs a target whose suite binds a shared fixture", func(w *gotest.T) {
		run, markers := runFuzzCLI(w, "fuzz", "--for=10s", "--target=FuzzSharedTestSuite_FuzzRead")
		w.It("starts the fixture once, its fuzz workers see its state, and it is torn down after", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "shared up", markers[0], "markers: %v", markers)
			gotest.Equal(it, "shared down", markers[len(markers)-1], "markers: %v", markers)
			gotest.Equal(it, 1, strings.Count(strings.Join(markers, "\n"), "shared up"), "markers: %v", markers)
			gotest.Contains(it, markers, "body saw up")
			gotest.NotContains(it, markers, "body saw ")
		})
	})
}

func (s *FuzzSharedFixtureTestSuite) TestFixtureDown(t *gotest.T) {
	t.When("gotest fuzz runs a target whose suite binds a shared fixture that does not come up", func(w *gotest.T) {
		run, markers := runFuzzCLIOn(w, "../testdata/fzsharedfail/", "fuzz", "--for=10s")
		w.It("fails without running the target, naming the fixture", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "FuzzDownTestSuite_FuzzRead never ran: shared fixture DownSharedFixture did not come up")
			gotest.Empty(it, markers)
		})
	})
}

func (s *FuzzSharedFixtureTestSuite) TestTriage(t *gotest.T) {
	t.When("gotest fuzz triage re-runs a crasher of a target whose suite binds a shared fixture", func(w *gotest.T) {
		run, markers := runFuzzCLI(w, "fuzz", "triage")
		w.It("reports the crasher's own failure, seen with the fixture's state, and tears the fixture down", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stdout, "cause: ")
			gotest.Contains(it, run.Stdout, "crashed on boom with the fixture up")
			gotest.Equal(it, []string{"shared up", "shared down"}, markers)
		})
	})
}
