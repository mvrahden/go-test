package lifecycle_test

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedStopTestSuite: a run that ends early, stopped or cut short by
// FailFast, still tears its shared fixture down before gotest exits.
type SharedStopTestSuite struct {
	cli string
}

func (s *SharedStopTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 2 * time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *SharedStopTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *SharedStopTestSuite) AfterAll(t *gotest.T) {}

// lcsShared is the package every run here selects one suite of.
const lcsShared = "../testdata/lcsshared/"

// lcsCommand is the CLI over one suite of lcsshared, the arguments before
// the package, the -run selection after it.
func lcsCommand(cli, markerDir, suite string, env, args []string) *exec.Cmd {
	args = append(append(args, lcsShared), "-run=^Test"+suite+"$")
	cmd := exec.Command(cli, args...) //nolint:gosec // G204: binary built by the suite
	cmd.Env = testkit.ChildEnv(markerDir, env...)
	return cmd
}

// lcsEnded waits up to within for the CLI and reads back what it left, and
// how long it took from since; one still running then is killed first, and
// its Exit is -2.
func lcsEnded(cmd *exec.Cmd, tree *proctree.Tree, markerDir string, since time.Time, within time.Duration) (testkit.Run, time.Duration) {
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	hung := false
	select {
	case <-done:
	case <-time.After(within):
		hung = true
		_ = tree.Kill()
		<-done
	}
	took := time.Since(since)
	tree.Release()
	run := testkit.Run{Exit: -2}
	if !hung && cmd.ProcessState != nil {
		run.Exit = cmd.ProcessState.ExitCode()
	}
	run.Stdout = cmd.Stdout.(*strings.Builder).String()
	run.Stderr = cmd.Stderr.(*strings.Builder).String()
	run.Events = testkit.ParseEvents([]byte(run.Stdout))
	run.Markers, _ = testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
	return run, took
}

// setupPID is the shared fixture process's pid, as its BeforeAll marked it.
func setupPID(markers []string) int {
	for _, m := range markers {
		if n, ok := strings.CutPrefix(m, "setup pid "); ok {
			pid, _ := strconv.Atoi(n)
			return pid
		}
	}
	return 0
}

// pipelineRow names a pipeline by the arguments that select it.
type pipelineRow struct {
	Desc string
	args []string
}

func (s *SharedStopTestSuite) TestInterruptedOnEveryPipeline(t *gotest.T) {
	t.When("the run is interrupted on the text, -json or spec pipeline while a method bound to a shared fixture ignores its context", func(w *gotest.T) {
		w.It("exits 130 and runs the shared fixture's AfterAll before gotest exits, on every pipeline", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []pipelineRow{
				{Desc: "text", args: nil},
				{Desc: "-json", args: []string{"-json"}},
				{Desc: "spec", args: []string{"spec", "--no-color"}},
			}) {
				markerDir := sub.TempDir()
				cmd := lcsCommand(s.cli, markerDir, "IgnoreTestSuite", nil, row.args)
				tree := reached(sub, cmd, markerDir, "running")
				signalled(sub, cmd, tree, testkit.Interrupt(cmd, tree))
				run, _ := lcsEnded(cmd, tree, markerDir, time.Now(), 60*time.Second)
				gotest.Equal(sub, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Markers, "shared.AfterAll ends", "stderr: %s", run.Stderr)
			}
		})
	})
}

func (s *SharedStopTestSuite) TestAFailFastSuiteBoundToASharedFixture(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), lcsShared, "-run=^TestFailFastTestSuite$")
	gotest.NoError(t, err)

	t.When("the first method of a FailFast suite bound to a shared fixture fails", func(w *gotest.T) {
		w.It("skips the method after it and runs the shared fixture's AfterAll before gotest exits", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "skip", run.Verdict("TestFailFastTestSuite/TestSecond"), "stream:\n%s", run.Stdout)
			gotest.NotContains(it, run.Markers, "second ran")
			gotest.Contains(it, run.Markers, "shared.AfterAll ends", "stderr: %s", run.Stderr)
		})
	})
}

func (s *SharedStopTestSuite) TestInterruptedWhileTheSharedFixturesTearDown(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := lcsCommand(s.cli, markerDir, "QuickTestSuite", []string{"LCS_TEARDOWN_SLEEP=3s"}, []string{"-json"})
	tree := reached(t, cmd, markerDir, "shared.AfterAll begins")
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	run, _ := lcsEnded(cmd, tree, markerDir, time.Now(), 60*time.Second)

	t.When("a passing run is interrupted while its shared fixture's AfterAll runs", func(w *gotest.T) {
		w.It("exits 0 and lets the AfterAll finish", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "shared.AfterAll ends", "stderr: %s", run.Stderr)
		})
	})
}
