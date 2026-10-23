package lifecycle_test

import (
	"os/exec"
	"path/filepath"
	"regexp"
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
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline, SetupTimeout: gotest.NoDeadline}
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
// output and markers.
func finished(cmd *exec.Cmd, tree *proctree.Tree, markerDir string) testkit.Run {
	err := cmd.Wait()
	tree.Release()
	run := testkit.Run{Exit: cmd.ProcessState.ExitCode()}
	if err != nil && run.Exit == 0 {
		run.Exit = -1
	}
	run.Stdout = cmd.Stdout.(*strings.Builder).String()
	run.Stderr = cmd.Stderr.(*strings.Builder).String()
	run.Markers, _ = testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
	return run
}

// The stop runs read go test -v's text, not -json: the runner signals the
// converter in front of a suite along with it, and the stream ends there.

// textVerdict is the verdict -v printed for the test at path: pass, fail,
// skip, or empty when none was printed.
func textVerdict(out, path string) string {
	m := regexp.MustCompile(`--- (PASS|FAIL|SKIP): ` + regexp.QuoteMeta(path) + ` \(`).FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// textOutput is what -v printed under the test at path, between its RUN line
// and the next RUN or verdict line.
func textOutput(out, path string) string {
	_, rest, found := strings.Cut(out, "=== RUN   "+path+"\n")
	if !found {
		return ""
	}
	var b strings.Builder
	for line := range strings.Lines(rest) {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "=== ") || strings.HasPrefix(trimmed, "--- ") {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func (s *StopTestSuite) TestAnInterruptStopsTheRun(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-v", "../testdata/stopping/") //nolint:gosec // G204: binary built by this suite
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
				gotest.Equal(it, "skip", textVerdict(run.Stdout, path), "verdict of %s:\n%s", path, run.Stdout)
				gotest.Contains(it, textOutput(run.Stdout, path), "run stopping")
			}
			gotest.NotContains(it, run.Markers, "second")
			gotest.NotContains(it, run.Markers, "third")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
	})
}
