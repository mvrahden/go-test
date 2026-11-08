//go:build unix

package lifecycle_test

import (
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ControlInputTestSuite: a run started with --control=stdin is stopped and
// forced by lines on its standard input, and by its end, counted with the
// signals as one source; without the flag its standard input is left alone.
// Sequential: it builds the CLI once and every method drives a live process.
type ControlInputTestSuite struct {
	cli string
}

func (s *ControlInputTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *ControlInputTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *ControlInputTestSuite) AfterAll(t *gotest.T) {}

// controlled starts a -json run of the stopping package, with --control=stdin
// when control is set, and returns once its first method holds, with the
// write end of its standard input.
func (s *ControlInputTestSuite) controlled(t *gotest.T, markerDir string, control bool, env ...string) (*exec.Cmd, *proctree.Tree, io.WriteCloser) {
	args := []string{"-json", "../testdata/stopping/"}
	if control {
		args = append([]string{"--control=stdin"}, args...)
	}
	cmd := exec.Command(s.cli, args...) //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir, env...)
	in, err := cmd.StdinPipe()
	gotest.NoError(t, err)
	return cmd, holding(t, cmd, markerDir), in
}

// send writes line to the run's standard input; a run it did not reach is
// killed before the test fails, so nothing outlives it.
func send(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, in io.Writer, line string) {
	_, err := io.WriteString(in, line+"\n")
	signalled(t, cmd, tree, err)
}

// controlBound is how long a stopped run gets to end; a stop no line
// brought leaves it holding until then.
const controlBound = 20 * time.Second

func (s *ControlInputTestSuite) TestAStopLine(t *gotest.T) {
	markerDir := t.TempDir()
	cmd, tree, in := s.controlled(t, markerDir, true)
	send(t, cmd, tree, in, "stop")
	run := ended(cmd, tree, markerDir, controlBound)

	t.When("a stop line reaches gotest's standard input while a method holds", func(w *gotest.T) {
		w.It("exits 130 and finishes the fixture teardown", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "context ended")
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "teardown done"), "markers: %v", run.Markers)
			gotest.NotContains(it, run.Stderr, "interrupted again")
		})
		w.It("books the run as interrupted on its stream", func(it *gotest.T) {
			gotest.Equal(it, "fail", packageVerdict(&run, "interrupted"), "stream:\n%s", run.Stdout)
		})
	})
}

// forcedRun reads a run forced while its method ignored the stop: released at
// once, without AfterAll, well before the five-second grace a single stop
// gives the method.
func forcedRun(it *gotest.T, run *testkit.Run, took time.Duration) {
	gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	gotest.Contains(it, run.Markers, "teardown done", "stderr: %s", run.Stderr)
	gotest.NotContains(it, run.Markers, "suite.AfterAll")
	gotest.Contains(it, run.Stderr, "interrupted again")
	gotest.Less(it, took, 4*time.Second)
	pid := pidOf(run.Markers)
	gotest.NotZero(it, pid, "markers: %v", run.Markers)
	gotest.True(it, gone(pid), "the suite process %d outlived the run", pid)
}

func (s *ControlInputTestSuite) TestAForceLineAfterAStop(t *gotest.T) {
	markerDir := t.TempDir()
	cmd, tree, in := s.controlled(t, markerDir, true, "STOPPING_IGNORE_CONTEXT=1")
	start := time.Now()
	send(t, cmd, tree, in, "stop")
	send(t, cmd, tree, in, "force")
	run := ended(cmd, tree, markerDir, controlBound)
	took := time.Since(start)

	t.When("a force line follows a stop line at once while the stopped method body ignores its context", func(w *gotest.T) {
		w.It("releases the fixture at once, without AfterAll, exits 130 before a single stop's five-second grace is over, saying so, and leaves no suite process behind", func(it *gotest.T) {
			forcedRun(it, &run, took)
		})
	})
}

func (s *ControlInputTestSuite) TestAForceLineAlone(t *gotest.T) {
	markerDir := t.TempDir()
	cmd, tree, in := s.controlled(t, markerDir, true, "STOPPING_IGNORE_CONTEXT=1")
	start := time.Now()
	send(t, cmd, tree, in, "force")
	run := ended(cmd, tree, markerDir, controlBound)
	took := time.Since(start)

	t.When("a force line reaches gotest with no stop under way while the method body ignores its context", func(w *gotest.T) {
		w.It("stops the run and forces the stop as a stop line followed by a force line does", func(it *gotest.T) {
			forcedRun(it, &run, took)
		})
	})
}

func (s *ControlInputTestSuite) TestTheInputEnds(t *gotest.T) {
	markerDir := t.TempDir()
	cmd, tree, in := s.controlled(t, markerDir, true)
	signalled(t, cmd, tree, in.Close())
	run := ended(cmd, tree, markerDir, controlBound)

	t.When("gotest's standard input ends while a method holds", func(w *gotest.T) {
		w.It("stops the run as a SIGINT does: exit 130, the holding method's context ended and the fixture teardown finished", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "teardown done"), "markers: %v", run.Markers)
			gotest.Equal(it, "fail", packageVerdict(&run, "interrupted"), "stream:\n%s", run.Stdout)
		})
	})
}

func (s *ControlInputTestSuite) TestWithoutTheFlag(t *gotest.T) {
	markerDir := t.TempDir()
	cmd, tree, in := s.controlled(t, markerDir, false, "STOPPING_HOLD=3s")
	send(t, cmd, tree, in, "stop")
	signalled(t, cmd, tree, in.Close())
	run := ended(cmd, tree, markerDir, controlBound)

	t.When("gotest runs without --control and its standard input carries a stop line, then ends", func(w *gotest.T) {
		w.It("runs to its end, its exit the tests' own", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "held")
			gotest.NotContains(it, run.Markers, "context ended")
			gotest.Contains(it, run.Markers, "third")
			gotest.Empty(it, packageVerdict(&run, "interrupted"))
		})
	})
}

type controlSignalRow struct {
	Desc  string
	after time.Duration // from the stop line to the SIGINT
}

func (s *ControlInputTestSuite) TestAStopLineAndASIGINT(t *gotest.T) {
	t.When("a SIGINT follows a stop line", func(w *gotest.T) {
		w.It("is the same stop within 300ms, finishing the fixture teardown, and later the second, abandoning the teardown saying so", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []controlSignalRow{
				{Desc: "at once, the same stop", after: 0},
				{Desc: "600ms later, while the fixture tears down, the second", after: 600 * time.Millisecond},
			}) {
				markerDir := sub.TempDir()
				// A teardown slow enough to outlast the second signal's kill.
				cmd, tree, in := s.controlled(sub, markerDir, true, "STOPPING_SLOW=6s")
				first := time.Now()
				send(sub, cmd, tree, in, "stop")
				if row.after > 0 {
					awaitMarkers(markerDir, 30*time.Second, "fixture.AfterAll")
					time.Sleep(time.Until(first.Add(row.after)))
				}
				signalled(sub, cmd, tree, syscall.Kill(-cmd.Process.Pid, syscall.SIGINT))
				run := ended(cmd, tree, markerDir, controlBound)

				gotest.Equal(sub, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				if row.after > 0 {
					gotest.Contains(sub, run.Stderr, "interrupted again")
					gotest.NotContains(sub, run.Markers, "teardown done")
				} else {
					gotest.NotContains(sub, run.Stderr, "interrupted again")
					gotest.Contains(sub, run.Markers, "teardown done")
				}
			}
		})
	})
}
