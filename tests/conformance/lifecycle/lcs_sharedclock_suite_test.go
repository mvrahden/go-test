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

// SharedStopClockTestSuite: a second interrupt, or an expired --timeout,
// ends a run bound to a shared fixture without leaving the fixture behind.
// Exclusive: its verdicts time a signal and a --timeout against the wall
// clock.
// Sequential: every method drives a live CLI the clock judges.
type SharedStopClockTestSuite struct {
	cli string
}

func (s *SharedStopClockTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 2 * time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *SharedStopClockTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	// A warm build, so no run here waits for one.
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), lcsShared, "-run=^TestQuickTestSuite$")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *SharedStopClockTestSuite) AfterAll(t *gotest.T) {}

// awaitMarker returns once marker is in markerDir; a run that never stamps
// it is killed before the test fails, so nothing outlives it.
func awaitMarker(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, markerDir, marker string) {
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(testkit.Scaled(90 * time.Second)); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, marker) {
			return
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never reached %s:\n%s%s", marker, cmd.Stdout, cmd.Stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
}

// secondInterrupt interrupts the run again, no sooner than a person's second
// Ctrl-C after the first, sent at first: a repeat sooner is the first
// delivered again.
func secondInterrupt(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, first time.Time) time.Time {
	time.Sleep(time.Until(first.Add(time.Second)))
	sent := time.Now()
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	return sent
}

func (s *SharedStopClockTestSuite) TestASecondInterruptDuringTheSharedTeardown(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := lcsCommand(s.cli, markerDir, "StoppableTestSuite", []string{"LCS_TEARDOWN_SLEEP=20s"}, []string{"-json"})
	tree := reached(t, cmd, markerDir, "running")
	first := time.Now()
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	awaitMarker(t, cmd, tree, markerDir, "shared.AfterAll begins")
	sent := secondInterrupt(t, cmd, tree, first)
	run, took := lcsEnded(cmd, tree, markerDir, sent, testkit.Scaled(10*time.Second))

	t.When("a second SIGINT reaches gotest while a shared fixture's 20s AfterAll runs", func(w *gotest.T) {
		w.It("exits 130 within 5s, leaving no shared fixture process behind 2s later", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Less(it, took, testkit.Scaled(5*time.Second))
			pid := setupPID(run.Markers)
			gotest.NotZero(it, pid, "markers: %v", run.Markers)
			gotest.True(it, testkit.EndedWithin(pid, testkit.Scaled(2*time.Second)), "the shared fixture process %d outlived the CLI", pid)
		})
	})
}

func (s *SharedStopClockTestSuite) TestASecondInterruptWhileAMethodIgnoresTheFirst(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := lcsCommand(s.cli, markerDir, "HeldTestSuite", nil, []string{"-json"})
	tree := reached(t, cmd, markerDir, "running")
	first := time.Now()
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	// Past the window of a repeat, well inside the method's 5s grace.
	sent := secondInterrupt(t, cmd, tree, first)
	run, _ := lcsEnded(cmd, tree, markerDir, sent, testkit.Scaled(10*time.Second))

	t.When("a second SIGINT reaches gotest while a stopped method body bound to a shared fixture ignores its context", func(w *gotest.T) {
		w.It("exits 130 once the shared fixture's AfterAll ran, leaving no shared fixture process behind", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "shared.AfterAll ends", "stderr: %s", run.Stderr)
			pid := setupPID(run.Markers)
			gotest.NotZero(it, pid, "markers: %v", run.Markers)
			gotest.True(it, testkit.EndedWithin(pid, testkit.Scaled(2*time.Second)), "the shared fixture process %d outlived the CLI", pid)
		})
	})
}

// timeoutRow is one pipeline a --timeout cuts short, with the unit its FAIL
// line names and a line only its stream shows.
type timeoutRow struct {
	Desc    string
	args    []string
	running string
	shows   string
}

func (s *SharedStopClockTestSuite) TestATimeoutOnEveryPipeline(t *gotest.T) {
	const pkg = "github.com/mvrahden/go-test/tests/conformance/testdata/lcsshared"
	// A warm run reaches the method in about a second.
	budget := testkit.Scaled(10 * time.Second)

	t.When("--timeout expires while a method bound to a shared fixture sleeps, on the text, -json or spec pipeline", func(w *gotest.T) {
		w.It("exits 1 naming in one FAIL line what was running, the method on -json and spec and the suite in the text run, and runs the shared fixture's AfterAll", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []timeoutRow{
				{Desc: "text", args: nil, running: " TestIgnoreTestSuite\n", shows: "FAIL\t" + pkg},
				{Desc: "-json", args: []string{"-json"}, running: " TestIgnoreTestSuite/TestSleeps\n", shows: `"Action":"fail","Package":"` + pkg + `","Test":"TestIgnoreTestSuite/TestSleeps"`},
				{Desc: "spec", args: []string{"spec", "--no-color"}, running: " TestIgnoreTestSuite/TestSleeps\n", shows: "✗ Sleeps"},
			}) {
				markerDir := sub.TempDir()
				cmd := lcsCommand(s.cli, markerDir, "IgnoreTestSuite", nil, append(row.args, "--timeout="+budget.String()))
				cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
				cmd.WaitDelay = 15 * time.Second
				tree, err := testkit.StartCLI(cmd)
				gotest.NoError(sub, err)
				run, _ := lcsEnded(cmd, tree, markerDir, time.Now(), budget+time.Minute)
				out := run.Stdout + run.Stderr
				gotest.Contains(sub, run.Markers, "running", "the --timeout expired before the method ran:\n%s", out)
				gotest.Equal(sub, 1, run.Exit, out)
				gotest.Contains(sub, out, "FAIL: global --timeout exceeded after "+budget.String()+" while running: "+pkg+row.running)
				gotest.Contains(sub, out, row.shows)
				gotest.NotContains(sub, out, "=== global --timeout")
				gotest.NotContains(sub, out, "census")
				gotest.Contains(sub, run.Markers, "shared.AfterAll ends", out)
			}
		})
	})
}
