//go:build unix

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StopLineTestSuite: the runner stops a suite by a line on the channel the
// suite inherited, the suite reports its teardown budget on it, and the
// channel stays with the suite process; a signal the runner did not send
// is a stop of its own.
type StopLineTestSuite struct {
	cli string
}

// SuiteConfig: a budget run waits out a 32s teardown budget or a 40s linger
// after a cold build; both budgets hold twice that.
func (s *StopLineTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *StopLineTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *StopLineTestSuite) AfterAll(t *gotest.T) {}

// stoppingPkg is the import path of the stopping package's events.
const stoppingPkg = "github.com/mvrahden/go-test/tests/conformance/testdata/stopping"

func (s *StopLineTestSuite) TestTheRunnersStop(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := commandOf(s.cli, "-json", "../testdata/stopping/")
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := holding(t, cmd, markerDir)
	began := time.Now()
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	run := finished(cmd, tree, markerDir)
	took := time.Since(began)

	t.When("the runner stops a suite whose method holds", func(w *gotest.T) {
		w.It("sends it no signal: the run's stderr names none", func(it *gotest.T) {
			// The method's own 30s deadline would end its context too.
			gotest.Less(it, took, 10*time.Second, "the stop never reached the suite")
			gotest.Contains(it, run.Markers, "context ended", "markers: %v", run.Markers)
			gotest.NotContains(it, run.Stderr, "terminated by signal")
			gotest.NotContains(it, run.Stdout+run.Stderr, "from outside the runner")
		})
		w.It("ends the -json stream whole, a verdict for every test in it", func(it *gotest.T) {
			for _, path := range []string{"TestStopTestSuite", "TestStopTestSuite/TestHolds", "TestStopTestSuite/TestSecond", "TestStopTestSuite/TestThird"} {
				gotest.NotEmpty(it, run.Verdict(path), "verdict of %s; stream:\n%s", path, run.Stdout)
			}
			gotest.Equal(it, "pass", packageVerdict(&run, stoppingPkg), "stream:\n%s", run.Stdout)
		})
	})
}

// budgetRun runs the stopbudget package with env, interrupts it once its
// method holds, and returns what the run left and how long it took to end
// after the interrupt. A run still going 90s after it is killed.
func (s *StopLineTestSuite) budgetRun(t *gotest.T, env ...string) (testkit.Run, time.Duration) {
	markerDir := t.TempDir()
	cmd := commandOf(s.cli, "-json", "../testdata/stopbudget/")
	cmd.Env = testkit.ChildEnv(markerDir, env...)
	tree := holding(t, cmd, markerDir)
	began := time.Now()
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	run := ended(cmd, tree, markerDir, 90*time.Second)
	return run, time.Since(began)
}

func (s *StopLineTestSuite) TestAShortTeardownBudget(t *gotest.T) {
	run, took := s.budgetRun(t, "STOPBUDGET_LINGER=75s")
	t.When("the runner stops a suite that reported a 32s teardown budget and lingers past it", func(w *gotest.T) {
		w.It("kills the suite once that budget is spent, not the default's", func(it *gotest.T) {
			gotest.NotContains(it, run.Markers, "lingered", "the suite outlived its budget; stderr: %s", run.Stderr)
			gotest.Equal(it, 130, run.Exit, "stderr: %s", run.Stderr)
			gotest.GreaterOrEqual(it, took, 30*time.Second)
			gotest.Less(it, took, 60*time.Second)
		})
	})
}

func (s *StopLineTestSuite) TestALongTeardownBudget(t *gotest.T) {
	run, took := s.budgetRun(t, "STOPBUDGET_LINGER=40s", "STOPBUDGET_TIMEOUT=2m")
	t.When("the runner stops a suite that reported a teardown budget longer than its 40s linger", func(w *gotest.T) {
		w.It("lets the suite end on its own", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "lingered", "the suite was killed after %s; stderr: %s", took, run.Stderr)
			gotest.NotContains(it, run.Stderr, "terminated by signal")
		})
	})
}

func (s *StopLineTestSuite) TestTheChannelStaysWithTheSuite(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := commandOf(s.cli, "-json", "../testdata/channel/")
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := started(t, cmd, markerDir)
	run := ended(cmd, tree, markerDir, 90*time.Second)
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)

	t.When("a test under the runner starts a process of its own", func(w *gotest.T) {
		w.It("the process inherits no descriptor of the channel", func(it *gotest.T) {
			// One socket fewer than the suite's own: the channel; the host may
			// hand every process other sockets.
			gotest.Equal(it, socketCount(run.Markers, "suite sockets=")-1, socketCount(run.Markers, "sockets="), "markers: %v", run.Markers)
		})
	})
	t.When("a process inherits the channel's variable naming a descriptor that is a regular file", func(w *gotest.T) {
		w.It("leaves that descriptor open", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "descriptor kept", "markers: %v", run.Markers)
		})
	})
	t.When("the last suite of a process under the runner ends", func(w *gotest.T) {
		w.It("leaves no goroutine reading the channel for a leak check in TestMain to find", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "no reader left", "markers: %v", run.Markers)
		})
	})
}

// inTestMain starts a run of the stopatopen package and returns once both
// its suite processes run their TestMain. A run that never gets there is
// killed before the test fails, so nothing outlives it.
func inTestMain(t *gotest.T, cmd *exec.Cmd, markerDir string) *proctree.Tree {
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if countOf(got, "main") == 2 {
			return tree
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the suite processes never ran their TestMain:\nmarkers: %v\n%s%s", got, cmd.Stdout, cmd.Stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
}

func (s *StopLineTestSuite) TestAStopDuringTestMain(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := commandOf(s.cli, "-json", "../testdata/stopatopen/")
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := inTestMain(t, cmd, markerDir)
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	// The stop is on its way to the suites; then their TestMain goes on.
	time.Sleep(time.Second)
	f, err := os.OpenFile(filepath.Join(markerDir, testkit.MarkersFile), os.O_APPEND|os.O_WRONLY, 0o600)
	signalled(t, cmd, tree, err)
	_, err = f.WriteString("go\n")
	_ = f.Close()
	signalled(t, cmd, tree, err)
	run := finished(cmd, tree, markerDir)

	t.When("the run is interrupted while its suite processes run their TestMain, before their first suite opens", func(w *gotest.T) {
		w.It("skips every suite with the reason run stopping", func(it *gotest.T) {
			for _, suite := range []string{"TestFirstTestSuite", "TestSecondTestSuite"} {
				gotest.Equal(it, "skip", run.Verdict(suite), "verdict of %s; stream:\n%s", suite, run.Stdout)
				gotest.Contains(it, outputOf(&run, suite), "run stopping")
			}
		})
		w.It("sets up no fixture", func(it *gotest.T) {
			for _, marker := range []string{"fixture.BeforeAll", "first.BeforeAll", "second.BeforeAll", "first ran", "second ran"} {
				gotest.NotContains(it, run.Markers, marker)
			}
		})
	})
}

// outsideRun runs the stopping package with env, sends its suite process a
// SIGTERM once a method holds and, when then is set, calls it after; it
// returns what the run left and how long it took to end after the signal.
func (s *StopLineTestSuite) outsideRun(t *gotest.T, then func(markers string) error, env ...string) (testkit.Run, time.Duration) {
	markerDir := t.TempDir()
	cmd := commandOf(s.cli, "-json", "../testdata/stopping/")
	cmd.Env = testkit.ChildEnv(markerDir, env...)
	tree := started(t, cmd, markerDir, "running", "pid=")
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	began := time.Now()
	signalled(t, cmd, tree, signalSuite(markers))
	if then != nil {
		signalled(t, cmd, tree, then(markers))
	}
	run := ended(cmd, tree, markerDir, 60*time.Second)
	return run, time.Since(began)
}

func (s *StopLineTestSuite) TestASignalFromOutside(t *gotest.T) {
	run, took := s.outsideRun(t, nil)
	t.When("a SIGTERM from outside the runner reaches a suite while a method holds", func(w *gotest.T) {
		w.It("ends the holding method's context and runs AfterAll before the fixture is released", func(it *gotest.T) {
			// The method's own 30s deadline would end its context too.
			gotest.Less(it, took, 10*time.Second, "the signal stopped nothing")
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v\nstderr: %s", run.Markers, run.Stderr)
		})
		w.It("names the stop in the stream as one from outside the runner", func(it *gotest.T) {
			gotest.Contains(it, packageOutput(&run, stoppingPkg), "gotest: suite stopped by SIGTERM from outside the runner", "stream:\n%s", run.Stdout)
		})
		w.It("fails the package and exits 1", func(it *gotest.T) {
			gotest.Equal(it, "fail", packageVerdict(&run, stoppingPkg), "stream:\n%s", run.Stdout)
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
		})
	})
}

func (s *StopLineTestSuite) TestASecondSignalDuringTheStop(t *gotest.T) {
	run, _ := s.outsideRun(t, func(markers string) error {
		// The holding method returns a second after its context ended: the
		// second signal lands while the stop waits for it.
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if got, _ := testkit.ReadMarkers(markers); countOf(got, "context ended") > 0 {
				break
			}
		}
		return signalSuite(markers)
	}, "STOPPING_RETURN_AFTER=1s")
	t.When("a second SIGTERM from outside the runner reaches the suite during the stop the first began", func(w *gotest.T) {
		w.It("still runs AfterAll before the fixture is released, the stop not forced", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v\nstderr: %s", run.Markers, run.Stderr)
			gotest.NotContains(it, run.Stderr, "torn down now")
		})
	})
}

// socketCount reads the number a marker with prefix carries, -1 without one.
func socketCount(markers []string, prefix string) int {
	for _, m := range markers {
		if n, err := strconv.Atoi(strings.TrimPrefix(m, prefix)); err == nil && strings.HasPrefix(m, prefix) {
			return n
		}
	}
	return -1
}
