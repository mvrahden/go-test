package lifecycle_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StopTestSuite: a run stopped from outside ends the method that holds, skips
// the methods that have not started, and tears down in order.
// Sequential: it builds the CLI once and every method drives a live process.
type StopTestSuite struct {
	cli string
}

func (s *StopTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *StopTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *StopTestSuite) AfterAll(t *gotest.T) {}

// holding starts a run of the stopping package through cmd and returns once
// its first method holds. A run that never holds is killed before the test
// fails, so nothing outlives it.
func holding(t *gotest.T, cmd *exec.Cmd, markerDir string) *proctree.Tree {
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, "running") {
			return tree
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never held:\n%s%s", cmd.Stdout, cmd.Stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
}

// signalled holds a signal's error; a run the signal did not reach is killed
// before the test fails, so nothing outlives it.
func signalled(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, err error) {
	if err == nil {
		return
	}
	_ = tree.Kill()
	_ = cmd.Wait()
	tree.Release()
	gotest.Fail(t, "signal: %v", err) //nolint:fail-guard // the tree is killed before the test fails
}

// finished waits for cmd and reads back what the run left: its exit code,
// events and markers.
func finished(cmd *exec.Cmd, tree *proctree.Tree, markerDir string) testkit.Run {
	err := cmd.Wait()
	tree.Release()
	run := testkit.Run{Exit: cmd.ProcessState.ExitCode()}
	if err != nil && run.Exit == 0 {
		run.Exit = -1
	}
	run.Stdout = cmd.Stdout.(*strings.Builder).String()
	run.Stderr = cmd.Stderr.(*strings.Builder).String()
	run.Events = testkit.ParseEvents([]byte(run.Stdout))
	run.Markers, _ = testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
	return run
}

func (s *StopTestSuite) TestAnInterruptStopsTheRun(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-json", "../testdata/stopping/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := holding(t, cmd, markerDir)
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	run := finished(cmd, tree, markerDir)

	t.When("the run is interrupted while a method holds", func(w *gotest.T) {
		w.It("exits 130 when gotest runs directly", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("ends the holding method's context", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "context ended")
		})
		w.It("skips the methods that have not started with the reason run stopping", func(it *gotest.T) {
			for _, method := range []string{"TestSecond", "TestThird"} {
				path := "TestStopTestSuite/" + method
				gotest.Equal(it, "skip", run.Verdict(path), "verdict of %s", path)
				var output strings.Builder
				for _, ev := range run.Events {
					if ev.Test == path && ev.Action == "output" {
						output.WriteString(ev.Output)
					}
				}
				gotest.Contains(it, output.String(), "run stopping")
			}
			gotest.NotContains(it, run.Markers, "second")
			gotest.NotContains(it, run.Markers, "third")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
	})
}
